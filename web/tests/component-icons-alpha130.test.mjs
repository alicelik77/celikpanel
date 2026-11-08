import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

// alpha.130 (owner, 8 Nov 2026): "the components do not have proper icons".
// Every component the server can report has an icon of its own, no two share
// one, none is a project's logo, and the Databases page draws the engines'
// marks instead of the server's emoji.
// Sunucunun bildirebileceği her bileşenin kendi ikonu var; ikisi aynı ikonu
// paylaşmaz; Veritabanları sayfası emoji yerine motor işaretini çizer.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const icons = read('../src/lib/componentIcons.tsx');
const marks = read('../src/lib/componentMarks.ts');
const catalogue = read('../../internal/core/managed_services.go');

const mapBody = icons.slice(icons.indexOf('const componentIconMap'));
const entries = new Map(
  [...mapBody.slice(0, mapBody.indexOf('};')).matchAll(/^\s+'?([a-z0-9-]+)'?: (\w+),$/gm)].map((m) => [m[1], m[2]]),
);

// The catalogue's components: the service rows' IDs (two tabs in), not the
// vendor repositories nested inside them (three tabs in).
const catalogueIDs = () => {
  const ids = [...catalogue.matchAll(/^\t\tID:\s+"([a-z0-9-]+)",/gm)].map((m) => m[1]);
  if (/^\t\tID:\s+NativeCronServiceID,/m.test(catalogue)) ids.push(catalogue.match(/const NativeCronServiceID = "([^"]+)"/)[1]);
  return ids;
};

test('every catalogue component has its own icon entry', () => {
  const ids = catalogueIDs();
  assert.ok(ids.length >= 30, `catalogue read: ${ids.length} components`);
  const missing = ids.filter((id) => !entries.has(id));
  assert.deepEqual(missing, [], 'a component would fall back to its category icon');
});

test('no two components share an icon', () => {
  const byIcon = new Map();
  for (const [id, icon] of entries) byIcon.set(icon, [...(byIcon.get(icon) ?? []), id]);
  const shared = [...byIcon].filter(([, ids]) => ids.length > 1);
  assert.deepEqual(shared, []);
});

test('no project logo motif: Apache feather and Node.js hexagon are gone', () => {
  assert.doesNotMatch(icons, /\bFeather\b/);
  assert.doesNotMatch(icons, /\bHexagon\b/);
});

test('our own marks are lucide line icons and import nothing else', () => {
  const imports = [...marks.matchAll(/from '([^']+)'/g)].map((m) => m[1]);
  assert.deepEqual(imports, ['lucide-react']);
  for (const [, name] of marks.matchAll(/export const (\w+): LucideIcon = createLucideIcon\('(\w+)'/g)) {
    assert.ok([...entries.values()].includes(name), `${name} is drawn on the Components page`);
  }
});

test('the Databases page draws the engine mark, not the server emoji', () => {
  const page = read('../src/components/DatabaseManagementV2.tsx');
  assert.doesNotMatch(page, /\{s\.type_icon\}/);
  assert.match(page, /<EngineMark engine=\{serverEngine\(s\.type_name\)\} size="sm" \/>/);
});
