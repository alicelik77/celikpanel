import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const settings = readFileSync(
  new URL('../src/components/Settings.tsx', import.meta.url),
  'utf8',
);
const english = englishCatalogue;
const turkish = turkishCatalogue;

function sourceBetween(startMarker, endMarker) {
  const start = settings.indexOf(startMarker);
  const end = settings.indexOf(endMarker, start + startMarker.length);
  assert.ok(start >= 0, `missing source marker: ${startMarker}`);
  assert.ok(end > start, `missing source marker after ${startMarker}: ${endMarker}`);
  return settings.slice(start, end);
}

function tabPanelSource(sectionID) {
  const marker = `id="settings-${sectionID}-panel"`;
  const start = settings.indexOf(marker);
  assert.ok(start >= 0, `missing ${sectionID} tab panel`);

  const panelEnd = settings.indexOf('</div>', start + marker.length);
  assert.ok(panelEnd > start, 'missing closing element for ' + sectionID + ' tab panel');
  return settings.slice(start, panelEnd + '</div>'.length);
}

test('software updates are a dedicated localized admin Settings section', () => {
  assert.match(settings, /type SettingsSectionID\s*=\s*[^;]*'updates'/s);

  const adminSections = sourceBetween("const sections: SettingsSection[] = role === 'admin'", ': [];');
  assert.match(adminSections, /id:\s*'updates'\s+as const/);
  assert.match(adminSections, /t\('settings\.section\.updates'\)/);
  assert.match(adminSections, /t\('settings\.section\.updates\.desc'\)/);

  for (const catalog of [english, turkish]) {
    assert.match(catalog, /'settings\.section\.updates':/);
    assert.match(catalog, /'settings\.section\.updates\.desc':/);
  }
});

test('panel access owns only the panel certificate while updates own PanelUpdateCard', () => {
  const panel = tabPanelSource('panel');
  const updates = tabPanelSource('updates');

  assert.match(panel, /<PanelCertificatePanel active=\{activeID === 'panel'\}\s*\/>/);
  assert.doesNotMatch(panel, /<PanelUpdateCard\s*\/>/);
  assert.match(updates, /<PanelUpdateCard\s*\/>/);
  assert.doesNotMatch(updates, /<PanelCertificatePanel\b/);
});

test('operational Settings components mount only while their URL section is active', () => {
  const updates = tabPanelSource('updates');
  const setup = tabPanelSource('setup');

  assert.match(
    updates,
    /hidden=\{activeID !== 'updates'\}[\s\S]*\{activeID === 'updates' && <Suspense[^>]*><PanelUpdateCard \/><\/Suspense>\}/,
  );
  assert.match(
    setup,
    /hidden=\{activeID !== 'setup'\}[\s\S]*\{activeID === 'setup' && <ServerSetupSettings \/>\}/,
  );
});

test('user section changes add history while invalid section canonicalization replaces it', () => {
  const canonicalization = sourceBetween('const requestedSection', 'const selectSection');
  assert.match(canonicalization, /if \(!activeSection \|\| requestedSection === activeSection\.id\) return/);
  assert.match(
    canonicalization,
    /setSearchParams\(\s*next\s*,\s*\{\s*replace:\s*true\s*\}\s*\)/s,
  );

  const selection = sourceBetween('const selectSection', 'const moveSection');
  assert.ok(selection.includes('if (!activeSection || section === activeSection.id) return;'));
  assert.match(
    selection,
    /setSearchParams\(\s*next(?:\s*,\s*\{\s*replace:\s*false\s*\})?\s*\)/s,
  );
  assert.doesNotMatch(selection, /replace:\s*true/);
});

test('the horizontally scrolling mobile tab list keeps the active section visible', () => {
  const workspace = sourceBetween('function SettingsWorkspace(', '// The certificate');

  assert.match(workspace, /overflow-x-auto/);
  assert.match(workspace, /tabRefs\.current\[section\.id\]\s*=\s*element/);
  assert.match(workspace, /const activeTab\s*=\s*tabRefs\.current\[activeID\]/);
  assert.match(workspace, /matchMedia\('\(max-width:\s*1023px\)'\)\.matches/);
  assert.match(workspace, /activeTab\.scrollIntoView\(\s*\{/);
  assert.match(workspace, /block:\s*'nearest'/);
  assert.match(workspace, /inline:\s*'nearest'/);
  assert.match(workspace, /\[activeID\]/);
});

// v0.1.0-alpha.98 (D-037, owner 2026-10-22): Settings holds only what is chosen
// once and decides how the panel behaves. The account, server security, DNS and
// the system databases have their own pages; no administrator-wide security
// policy exists, so Settings has four sections, in this order.
test('Settings shows only the four remaining sections, in the approved order', () => {
  const adminSections = sourceBetween("const sections: SettingsSection[] = role === 'admin'", ': [];');
  const ids = [...adminSections.matchAll(/id:\s*'([a-z-]+)'\s+as const/g)].map((match) => match[1]);
  assert.deepEqual(ids, ['panel', 'license', 'updates', 'setup']);
  assert.match(settings, /type SettingsSectionID = 'panel' \| 'license' \| 'updates' \| 'setup';/);
  for (const moved of ['account', 'security', 'dns', 'system-databases']) {
    assert.ok(!settings.includes(`id="settings-${moved}-panel"`), `Settings still draws the ${moved} section`);
  }
  for (const gone of ['TwoFactorPanel', 'SecurityAuditCard', 'DNSServerSettings', 'SystemSQLiteManager']) {
    assert.ok(!settings.includes(gone), `Settings still mounts ${gone}`);
  }
  for (const catalog of [english, turkish]) {
    for (const key of ['settings.section.account', 'settings.section.security', 'settings.section.dns', 'settings.section.systemDatabases']) {
      assert.ok(!catalog.includes(`'${key}'`), `${key} is still in a catalogue`);
    }
  }
  assert.match(english, /'settings\.section\.panel': 'Panel address and HTTPS'/);
  assert.match(turkish, /'settings\.section\.panel': 'Panel adresi ve HTTPS'/);
});
