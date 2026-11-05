import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

// alpha.125 "faster panel": the Components check runs in the background
// without freezing the page, a start/stop reads the state at once, polls
// pause in a hidden tab, and the Panel gzips the assets itself (not the build).
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const serviceList = read('../src/components/ServiceList.tsx');
const serviceShell = read('../src/components/ServiceShell.tsx');

test('a background check keeps the Components list usable', () => {
  // Only the first check of a server with no stored list holds the rows.
  assert.match(serviceList, /const pageControlsBusy = busy !== null \|\| \(scanning && scannedAt === null\);/);
  assert.doesNotMatch(serviceList, /const pageControlsBusy = scanning \|\| busy !== null;/);
  // The check buttons wait for the check they would start again.
  assert.match(serviceList, /const scanControlsBusy = scanning \|\| busy !== null;/);
  assert.equal((serviceList.match(/onClick=\{scan\}\s*disabled=\{scanControlsBusy\}/g) ?? []).length, 3);
  // Rows whose record did not change keep their object.
  assert.match(serviceList, /setServices\(\(previous\) => keepUnchangedRows\(previous, snapshot\.services\)\)/);
});

test('a check answer that raced a change is not applied, and a held refusal is no failure', () => {
  assert.match(serviceList, /if \(busy !== null \|\| runningOperation !== null\) changeEpochRef\.current \+= 1;/);
  assert.equal((serviceList.match(/if \(epoch !== changeEpochRef\.current\) return true;/g) ?? []).length, 2);
  assert.match(serviceList, /const SCAN_HELD_CODES = new Set\(\['server_setup_busy', 'service_operation_busy'\]\);/);
  assert.match(serviceList, /if \(storedScannedAt !== null && SCAN_HELD_CODES\.has\(error\.code \?\? ''\)\) return true;/);
});

test('a start or stop reads the state at once, without a fixed wait', () => {
  assert.doesNotMatch(serviceShell, /setTimeout\(res, 1500\)/);
});

test('timed reads pause while the tab is hidden', () => {
  for (const file of ['Dashboard', 'ServiceList', 'MonitoringPage', 'SecurityPage', 'DomainLogsViewer']) {
    const source = read(`../src/components/${file}.tsx`);
    assert.match(source, /import \{ startVisibleReads \} from '\.\.\/lib\/visibleReads';/, file);
    assert.doesNotMatch(source, /window\.setInterval\(\(\) => void (refresh|reread)\(\), (5000|READINESS_POLL_MS|REFRESH_MS)\)/, file);
  }
  const dashboard = read('../src/components/Dashboard.tsx');
  assert.doesNotMatch(dashboard, /setInterval\(loadStats, 5000\)|setInterval\(load, 5000\)/);
});

test('the build stores no gzip copies: the Panel compresses assets itself, the same on every host', () => {
  // Node's zlib may pick different matches on different CPUs, so build-time
  // .gz copies would make the release archive depend on the build host.
  const pkg = JSON.parse(read('../package.json'));
  assert.match(pkg.scripts.build, /node scripts\/check-bundle-budget\.mjs"?$/);
  assert.doesNotMatch(pkg.scripts.build, /precompress/);
  const compression = read('../../cmd/panel/compression.go');
  assert.match(compression, /gzip\.NewWriterLevel\(&packed, gzip\.BestCompression\)/);
  assert.match(compression, /var compressedAssets sync\.Map/);
});
