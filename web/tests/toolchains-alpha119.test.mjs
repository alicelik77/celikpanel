// alpha.119: developer toolchains (Go, Rust) in Components, for the server
// administrator alone. The catalogue (internal/core/managed_services.go) owns
// them; this file pins what the screen says about them and that the screen
// never treats them as something a customer can be offered.
//
// alpha.119: Bileşenler'de geliştirici araç zincirleri (Go, Rust), yalnız sunucu
// yöneticisi için.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const englishCatalogue = enScreens;
const turkishCatalogue = trScreens;

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const goCatalogue = read('../../internal/core/managed_services.go');
const serviceList = read('../src/components/ServiceList.tsx');
const icons = read('../src/lib/componentIcons.tsx');

// The catalogue's entry of one component, from its ID line to the next entry.
const entryOf = (id) => {
  const start = goCatalogue.indexOf(`\t\tID:          "${id}",`);
  assert.ok(start > 0, `no catalogue entry for ${id}`);
  return goCatalogue.slice(start, goCatalogue.indexOf('\n\t},', start));
};

test('the catalogue has Go and Rust as administrator-only tools from distribution packages', () => {
  const go = entryOf('go');
  const rust = entryOf('rust');
  for (const entry of [go, rust]) {
    assert.match(entry, /Category:\s+DeveloperToolsCategory,/);
    assert.match(entry, /Kind:\s+KindTool,/);
    assert.match(entry, /AdminOnly:\s+true,/);
    assert.doesNotMatch(entry, /Repo:/, 'a toolchain has no vendor repository');
  }
  assert.match(go, /"apt": \{"golang-go"\}, "pacman": \{"go"\}/);
  assert.match(rust, /"apt": \{"rustc", "cargo"\}, "pacman": \{"rust"\}/);
  assert.match(goCatalogue, /const DeveloperToolsCategory = "devtools"/);
});

test('the Components page has the developer tools card, in English and Turkish', () => {
  assert.match(serviceList, /id: 'devtools', labelKey: 'services\.cat\.devtools'/);
  assert.match(icons, /devtools: Wrench/);
  assert.equal(englishCatalogue['services.cat.devtools'], 'Developer tools (administrator only)');
  assert.equal(turkishCatalogue['services.cat.devtools'], 'Geliştirici araçları (yalnız yönetici)');
});

test('each toolchain row says whose it is, whose version it is, and where customers stand', () => {
  for (const id of ['go', 'rust']) {
    const en = englishCatalogue[`services.component.description.${id}`];
    const tr = turkishCatalogue[`services.component.description.${id}`];
    assert.ok(en && tr, `${id} needs an English and a Turkish description`);
    // The version is the distribution's and can be old.
    assert.match(en, /distribution’s own and can be older/);
    assert.match(tr, /dağıtımın kendi sürümüdür ve .* eski olabilir/);
    // The one-line explanation of the administrator-only rule.
    assert.match(en, /administrator’s own work on this server; customers’ programs are uploaded already built and run in a sandbox/);
    assert.match(tr, /Yöneticinin bu sunucudaki kendi işi içindir; müşteri programları derlenmiş yüklenir ve kum havuzunda çalışır/);
  }
  assert.equal(englishCatalogue['services.component.name.go'], 'Go');
  assert.equal(englishCatalogue['services.component.name.rust'], 'Rust');
  assert.equal(turkishCatalogue['services.component.name.go'], 'Go');
  assert.equal(turkishCatalogue['services.component.name.rust'], 'Rust');
});

test('no customer-facing screen part names a toolchain as something to offer', () => {
  for (const [name, catalogue] of [['en', englishCatalogue], ['tr', turkishCatalogue]]) {
    for (const key of Object.keys(catalogue)) {
      assert.ok(!/^(rights|plan|offer)[a-z.]*\.component\.(go|rust)\b/i.test(key), `${name}: ${key}`);
      assert.ok(!/component:(go|rust)\b/.test(String(catalogue[key])), `${name}: ${key} offers a toolchain`);
    }
  }
});
