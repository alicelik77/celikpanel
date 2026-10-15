// The server catalogue's components by name in the interface language (set16
// observation O2: the Turkish Components page showed "Firewall engine"). The
// catalogue ids are read from the Go catalogue itself (internal/core
// ManagedServices), so a component added there without an English and a
// Turkish display name fails here instead of showing English on a Turkish page.
//
// Sunucu kataloğundaki bileşenlerin arayüz dilindeki adları. Kimlikler Go
// kataloğundan okunur; adı olmadan eklenen bileşen burada kalır.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const compiled = ts.transpileModule(read('../src/lib/componentName.ts'), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const lib = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'));
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };
const say = (locale) => (key) => catalogues[locale][key] ?? key;

// The ManagedServices list, its top-level entries (two tabs deep; a
// repository's own ID is deeper), with constant ids resolved.
const goSource = read('../../internal/core/managed_services.go');
const listBody = /\nvar ManagedServices = \[\]ManagedService\{\n([\s\S]*?)\n\}\n/.exec(goSource)?.[1] ?? '';
const constants = Object.fromEntries([...goSource.matchAll(/^const (\w+) = "([a-z0-9-]+)"$/gm)].map((m) => [m[1], m[2]]));
const entries = [...listBody.matchAll(/^\t\tID:\s+(?:"([a-z0-9-]+)"|(\w+)),\n\t\tName:\s+"([^"]+)",\n\t\tDescription:\s+"([^"]+)",/gm)]
  .map((m) => ({ id: m[1] ?? constants[m[2]], name: m[3], description: m[4] }));

test('every catalogue component has an English and a Turkish display name', () => {
  assert.ok(entries.length >= 25, `read ${entries.length} components from the Go catalogue`);
  assert.ok(entries.every((entry) => entry.id), `an id did not resolve: ${JSON.stringify(entries)}`);
  assert.ok(entries.some((entry) => entry.id === 'cron'), 'the constant id of the cron entry was not resolved');
  for (const { id, name } of entries) {
    const key = `services.component.name.${id}`;
    assert.ok(catalogues.en[key], `no English name for ${id}`);
    assert.ok(catalogues.tr[key], `no Turkish name for ${id}`);
    // The English name is the catalogue's own (typographic apostrophe aside).
    assert.equal(catalogues.en[key], name.replaceAll("'", '’'), `${id}: the English name differs from the catalogue's`);
  }
  // No key for a component the catalogue does not have.
  const ids = new Set(entries.map((entry) => entry.id));
  for (const locale of ['en', 'tr']) {
    for (const key of Object.keys(catalogues[locale]).filter((item) => item.startsWith('services.component.name.'))) {
      assert.ok(ids.has(key.slice('services.component.name.'.length)), `${locale} names ${key}, which is not in the catalogue`);
    }
  }
});

test('the descriptive names are Turkish on a Turkish page; product names stay', () => {
  assert.equal(lib.componentName({ id: 'nftables', name: 'Firewall engine' }, say('tr')), 'Güvenlik duvarı motoru');
  assert.equal(lib.componentName({ id: 'nftables', name: 'Firewall engine' }, say('en')), 'Firewall engine');
  assert.equal(lib.componentName({ id: 'certbot', name: "Let's Encrypt client (certbot)" }, say('tr')), 'Let’s Encrypt istemcisi (certbot)');
  assert.equal(lib.componentName({ id: 'cron', name: 'Scheduled tasks (cron)' }, say('tr')), 'Zamanlanmış görevler (cron)');
  assert.equal(lib.componentName({ id: 'nginx', name: 'Nginx' }, say('tr')), 'Nginx');
  for (const id of ['nftables', 'certbot', 'cron']) {
    assert.notEqual(catalogues.tr[`services.component.name.${id}`], catalogues.en[`services.component.name.${id}`], `${id} is not translated`);
  }
});

test('a component this page has no name for keeps the server’s name, then its id', () => {
  assert.equal(lib.componentName({ id: 'from-a-newer-panel', name: 'Something new' }, say('tr')), 'Something new');
  assert.equal(lib.componentName({ id: 'from-a-newer-panel', name: '  ' }, say('tr')), 'from-a-newer-panel');
  assert.equal(lib.componentName('from-a-newer-panel', say('tr')), 'from-a-newer-panel');
  assert.equal(lib.componentName('nftables', say('tr')), 'Güvenlik duvarı motoru');
  assert.equal(lib.componentName({ id: 'Bad Id', name: 'Shown as sent' }, say('tr')), 'Shown as sent');
});

test('the component screens name a component through the catalogue words', () => {
  const list = read('../src/components/ServiceList.tsx');
  // Only the search reads the server's name, beside the translated one.
  const drawn = list.replace('`${componentName(s, t)} ${s.name} ${s.description} ${componentDescription(s, t)} ${s.id}`', '');
  assert.ok(!/\{(?:s|service)\.name\}|\$\{(?:s|service|action\.service)\.name\}/.test(drawn), 'the Components list draws the server name raw');
  assert.ok(!/name: (?:action\.)?service\.name\b/.test(list), 'a dialog or operation names the component by the server name');
  assert.ok(/componentName\(record, t\)/.test(read('../src/components/ComponentDetail.tsx')));
  assert.ok(!/row\.name\b|\?\.name \|\| id/.test(read('../src/components/ServerSetupComponents.tsx')));
  assert.ok(/svcStoppedItem', \{ name: componentName\(s, t\) \}/.test(read('../src/components/Dashboard.tsx')));
});

test('every catalogue component has an English and a Turkish description, and a page shows them', () => {
  for (const { id, description } of entries) {
    const key = `services.component.description.${id}`;
    assert.ok(catalogues.en[key], `no English description for ${id}`);
    assert.ok(catalogues.tr[key], `no Turkish description for ${id}`);
    assert.equal(catalogues.en[key], description.replaceAll("'", '’'), `${id}: the English description differs from the catalogue's`);
  }
  const ids = new Set(entries.map((entry) => entry.id));
  for (const locale of ['en', 'tr']) {
    for (const key of Object.keys(catalogues[locale]).filter((item) => item.startsWith('services.component.description.'))) {
      assert.ok(ids.has(key.slice('services.component.description.'.length)), `${locale} describes ${key}, which is not in the catalogue`);
    }
  }
  assert.equal(lib.componentDescription({ id: 'fail2ban', description: 'Intrusion Prevention' }, say('tr')), 'Saldırı önleme');
  assert.equal(lib.componentDescription({ id: 'fail2ban', description: 'Intrusion Prevention' }, say('en')), 'Intrusion Prevention');
  assert.equal(lib.componentDescription({ id: 'cron', description: 'x' }, say('tr')), 'Zamanlanmış görevleri çalıştıran yerel cron hizmeti');
  // Unknown component: the server's text, then nothing (never the id).
  assert.equal(lib.componentDescription({ id: 'from-a-newer-panel', description: 'Something new' }, say('tr')), 'Something new');
  assert.equal(lib.componentDescription({ id: 'from-a-newer-panel' }, say('tr')), '');
  assert.equal(lib.componentDescription({ id: 'Bad Id', description: 'Shown as sent' }, say('tr')), 'Shown as sent');
  const list = read('../src/components/ServiceList.tsx');
  assert.ok(!/(?<!\$)\{(?:s|service)\.description\}/.test(list), 'the Components list or a dialog draws the server description raw');
  assert.ok((list.match(/componentDescription\((?:s|service), t\)/g) ?? []).length >= 4, 'the list row, install, uninstall dialogs and search use the translated description');
});

// Browser batch 14 (2026-10-15): the Dashboard's "service is stopped" line led
// to the Components page with "Go to services" / "Servislere git", a page the
// menu does not have. The link names the page as the menu does.
// Panodaki bağlantı sayfayı menüdeki adıyla söyler.
test('the Dashboard link to the Components page names it as the menu does', () => {
  for (const locale of ['en', 'tr']) {
    const page = catalogues[locale]['nav.services'];
    assert.ok(catalogues[locale]['dashboard.goServices'].includes(page), `${locale}: "${catalogues[locale]['dashboard.goServices']}" does not name "${page}"`);
  }
});
