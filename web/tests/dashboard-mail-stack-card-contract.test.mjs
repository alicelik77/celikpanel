// The Dashboard's Ready bundles card (alpha.94, D-034). It replaced the Mail
// stacks card: one calm card, always shown, and a door to the bundles on the
// Components page. Since alpha.95 (owner decision, 2026-10-17) it is a compact
// door: one summary line counted as the Components chips are drawn, then only
// the lines that need a look. It installs nothing, scans nothing by itself and asks the
// owner for nothing (D-032).
// Panel'in Hazır paketler kartı: tek sakin kart, her zaman görünür; Bileşenler
// sayfasındaki paketlere bir kapı. Hiçbir şey kurmaz.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const dashboard = readFileSync(new URL('../src/components/Dashboard.tsx', import.meta.url), 'utf8');
const bundlesLib = readFileSync(new URL('../src/lib/readyBundles.ts', import.meta.url), 'utf8');
const services = readFileSync(new URL('../src/components/ServiceList.tsx', import.meta.url), 'utf8');
const en = englishCatalogue;
const tr = turkishCatalogue;
const card = dashboard.slice(dashboard.indexOf('function ReadyBundlesCard'), dashboard.indexOf('\n}\n', dashboard.indexOf('function ReadyBundlesCard')));

test('the bundles card is read-only, always shown and uses the closed bundle decoder', () => {
  assert.match(dashboard, /bundles: decodeReadyBundles\(payload\.bundles, serviceIDs\) \?\? \[\]/);
  assert.match(dashboard, /<ReadyBundlesCard\s/);
  // Always shown: no activity gate in front of it.
  assert.doesNotMatch(dashboard, /&& \(\s*\n\s*<ReadyBundlesCard/);
  assert.match(dashboard, /navigate\('\/services#bundles'\)/);
  assert.doesNotMatch(dashboard, /service\/profile\/install|startInstall|service\/install/);
  assert.match(card, /bundlesSummary\(bundles\)/);
  // One counting rule for the card and the chips: every registry entry,
  // add-ons included; "can be installed" is available or partial, a blocked
  // bundle is not counted; "installed and verified" includes a bundle a
  // verified bundle contains (Core Mail under Webmail).
  assert.match(bundlesLib, /export const bundleInstallable = \(state: BundleChipState\) => state === 'available' \|\| state === 'partial';/);
  assert.match(bundlesLib, /installable: states\.filter\(\(\{ state \}\) => bundleInstallable\(state\)\)\.length/);
  assert.match(bundlesLib, /verified: states\.filter\(\(\{ state \}\) => state === 'verified' \|\| state === 'included'\)\.length/);
  assert.match(bundlesLib, /const states = all\.map\(\(bundle\) => \(\{ bundle, state: bundleChipState\(bundle, all\) \}\)\);/);
  // A compact door: one summary line, then only what needs a look; no row
  // per verified bundle.
  assert.match(card, /data-bundles-summary=''/);
  // A bundle not verified with a failed or running attempt is one line, with the reason.
  assert.match(card, /summary\.unverified\.filter\(\(bundle\) => !\(bundle\.profile_id !== undefined && attempted\.has\(bundle\.profile_id\)\)\)\.map\(/);
  assert.doesNotMatch(card, /<ul|<li|summary\.installed/);
  // The old card and its gate are gone.
  assert.doesNotMatch(dashboard, /MailStackSummary|hasMailActivity|dashboard\.mailStacks|dashboardMailTruth/);
});

// D-024: a known mail failure is said again. The card names a failed or a
// running mail attempt with the Panel's own error (latest_attempt_error),
// whatever the scan's age; the mounted test in remote-state-mounted-batch3
// draws it.
test('a failed or running mail attempt is said again on the card, with its error', () => {
  assert.match(card, /profiles\.filter\(\(profile\) => profile\.latest_attempt_status === 'failed' \|\| profile\.latest_attempt_status === 'in_progress'\)/);
  assert.match(card, /t\('dashboard\.bundles\.attemptRunning', \{ name: mailProfileName\(profile, t\) \}\)/);
  assert.match(card, /reason: mailProfileAttemptError\(profile, t\) \|\| t\('services\.bundles\.lastAttemptFailed'\)/);
  assert.match(dashboard, /profiles=\{mailProfiles \?\? \[\]\}/);
});

test('the card is calm: no warning colour, one quiet door instead of a filled button', () => {
  assert.doesNotMatch(card, /warning/);
  assert.doesNotMatch(card, /bg-primary/);
  assert.match(card, /t\('dashboard\.bundles\.open'\)/);
  // No far-right "verified" label per row any more (owner's Frankfurt screen).
  assert.doesNotMatch(card, /ml-auto/);
});

test('the bundles destination scrolls and focuses the list once', () => {
  assert.match(services, /location\.hash !== '#bundles'/);
  assert.match(services, /getElementById\('bundles'\)/);
  assert.match(services, /scrollIntoView\(\{ block: 'start' \}\)/);
  assert.match(services, /focus\(\{ preventScroll: true \}\)/);
  assert.match(services, /id='bundles' tabIndex=\{-1\}/);
  // The old address is an alias that scrolls to the email strip.
  assert.match(services, /location\.hash === '#mail-stacks'\s*\n\s*\? \(document\.querySelector\('\[data-bundle-strip="email"\]'\)/);
});

test('dashboard bundles copy stays in EN/TR parity', () => {
  for (const key of [
    'dashboard.bundles.title',
    'dashboard.bundles.summary',
    'dashboard.bundles.summaryVerified',
    'dashboard.bundles.summaryInstallable',
    'dashboard.bundles.summaryNone',
    'dashboard.bundles.unverifiedLine',
    'dashboard.bundles.notChecked',
    'dashboard.bundles.checkFailed',
    'dashboard.bundles.attemptRunning',
    'dashboard.bundles.attemptFailed',
    'dashboard.bundles.open',
    'dashboard.bundles.cat.email',
    'dashboard.bundles.cat.database',
    'dashboard.bundles.cat.web',
  ]) {
    assert.ok(en.includes(`'${key}':`), 'missing EN key ' + key);
    assert.ok(tr.includes(`'${key}':`), 'missing TR key ' + key);
  }
  // The card never asks the owner to rescan by hand (D-032).
  for (const catalogue of [en, tr]) {
    for (const line of catalogue.split('\n').filter((entry) => entry.includes("'dashboard.bundles."))) {
      assert.doesNotMatch(line, /rescan|scan now|yeniden tara/i, line);
    }
    assert.ok(!catalogue.includes("'dashboard.mailStacks."), 'the mail stacks copy is gone');
  }
});
