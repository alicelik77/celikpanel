import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const dashboard = readFileSync(new URL('../src/components/Dashboard.tsx', import.meta.url), 'utf8');
const operation = readFileSync(new URL('../src/components/ComponentOperation.tsx', import.meta.url), 'utf8');
const layout = readFileSync(new URL('../src/components/Layout.tsx', import.meta.url), 'utf8');
const census = readFileSync(new URL('../src/lib/componentCensus.ts', import.meta.url), 'utf8');
const en = englishCatalogue;
const tr = turkishCatalogue;

test('mail journey requires fresh runtime state and completed profile reconciliation', () => {
  assert.match(operation, /typeof profile\.verified !== 'boolean'/);
  assert.match(operation, /profile\.verified && status !== 'complete'/);
  assert.match(dashboard, /const serviceScanFresh = freshScanTimestamp\(\s*serviceScan\.scannedAt,\s*serviceScan\.observedAt,\s*serviceScan\.scanAgeSeconds,\s*freshnessNow,\s*\)/);
  assert.match(dashboard, /profile\.status === 'complete'[\s\S]*profile\.verified[\s\S]*!profile\.warning/);
  assert.match(dashboard, /const mailProfileVerified = verifiedMailProfiles\.length > 0/);
  // alpha.94: the Mail stacks card gave way to one Ready bundles card,
  // always shown and read-only.
  assert.match(dashboard, /<ReadyBundlesCard/);
});

test('Boston rspamd tuple never produces a false SpamAssassin install alert', () => {
  assert.match(dashboard, /const hasSpam = serviceRunning\('spamassassin'\) \|\| serviceRunning\('rspamd'\)/);
  assert.match(dashboard, /if \(mailProfileVerified && !hasSpam\)/);
  assert.doesNotMatch(dashboard, /install SpamAssassin/);
});

test('system service truth never promotes tools into running daemons', () => {
  assert.match(dashboard, /if \(!serviceScanFresh\) return false/);
  assert.match(dashboard, /const systemServices = serviceScanFresh[\s\S]*installed\.filter\(\(s\) => s\.kind === 'service'\)/);
  // One running rule for every screen since alpha.98 (lib/serviceStatus.ts):
  // only a status that says running is running; "active (exited)" is not.
  assert.match(dashboard, /return isServiceRunning\(status\);/);
  const serviceStatus = readFileSync(new URL('../src/lib/serviceStatus.ts', import.meta.url), 'utf8');
  assert.match(serviceStatus, /return normalized === 'running' \|\| normalized\.includes\('\(running\)'\);/);
  assert.match(dashboard, /const hostNeverChecked = services\.length > 0 && uncheckedServices\.length === services\.length/);
  assert.match(dashboard, /!serviceScanFresh[\s\S]*dashboard\.statusUnknown/);
  assert.match(dashboard, /serviceScanFresh && hostsContent && !hasClamAV/);
  assert.match(dashboard, /attention\.length > 0 && \(/);
  assert.doesNotMatch(dashboard, /t\('dashboard\.allGood'\)/);
  // 9 Oct 2026: the attention section keeps its place instead of appearing
  // late and pushing the page down, so it has a line for "no items". That line
  // is not "all good". It is drawn only when every read the list is built from
  // has answered and none could not be read, it names what was read, and it
  // does not speak for components whose check is not current.
  // Since 12 Oct 2026 what it says of the components follows the
  // system-services card: being checked, could not be checked, waiting for a
  // change, or not checked recently.
  assert.match(dashboard,
    /\{!attentionChecking && !attentionUnread && attention\.length === 0 && \(\s*<p[^>]*>\s*\{t\(serviceScanFresh \? 'dashboard\.attentionNone' : attentionComponentsKey\)\}/);
  assert.match(dashboard,
    /const attentionComponentsKey: TranslationKey = servicesCard === 'checking'\s*\? 'dashboard\.attentionNoneChecking'\s*: servicesCard === 'failed'\s*\? 'dashboard\.attentionNoneCheckFailed'\s*: servicesCard === 'held'\s*\? 'dashboard\.attentionNoneCheckHeld'\s*: 'dashboard\.attentionNoneUnchecked';/);
  assert.match(dashboard, /const attentionChecking = attentionReads\.includes\('loading'\);/);
  assert.match(dashboard, /const attentionUnread = attentionReads\.includes\('unknown'\);/);
  assert.match(dashboard,
    /const attentionReads = \[\s*extrasRead\.remote\.state,\s*domainList\.remote\.state,\s*firewall\.remote\.state,\s*scanRead === 'reading' \|\| \(scanRead === 'unverified' && servicesCard === 'checking'\) \? 'loading'\s*: scanRead === 'known' \? 'known' : 'unknown',\s*\];/,
    'the calm line waits for the certificates, the domain list, the firewall and the component records');
  // The firewall is "off" only for an answer that says so: an answer without a
  // boolean `enabled`, or one carrying the Agent's own error, is unknown.
  assert.match(dashboard, /const firewall = useFirewallStatus\(\);/);
  assert.doesNotMatch(dashboard, /setFw\(|\.then\(setFw\)/, 'no answer is stored as the firewall state without being decoded');
  assert.match(census, /observed\.filter\(\(row\) => row\.is_installed === true\)\.length/);
});

test('dashboard preserves independent service and firewall evidence while setup has its own persisted state', () => {
  assert.match(dashboard, /typeof payload\.dns_identity_ready !== 'boolean'/);
  assert.match(dashboard, /<ServerSetupDashboardNotice/);
  assert.doesNotMatch(dashboard, /<StartGuide/);

  assert.match(dashboard, /fw\?\.enabled && fw\.persistence_state !== 'ready'/);
});

test('audit, alert and gauge fixes remain localized in EN and TR', () => {
  assert.match(dashboard, /audit-logs\?limit=28/);
  assert.match(dashboard, /groupAuditEntries\(audit\)\.slice\(0, 7\)/);
  assert.match(dashboard, /attention\.length === 1/);
  assert.match(dashboard, /dashboard\.percentValue/);
  assert.equal(dashboard.match(/t\('dashboard\.percentValue'/g)?.length, 6);
  for (const key of [
    'dashboard.warnCountOne',
    'dashboard.percentValue',
    'dashboard.loadValue',
    'dashboard.statusUnknown',
    'dashboard.step.serviceScan',
    'dashboard.fwPersistenceItem',
    'dashboard.audit.action.mail.profile.install.failed',
    'dashboard.step.dnsIdentity',
    'dashboard.saveFirewall',
  ]) {
    assert.ok(en.includes(`'${key}':`), 'missing EN key ' + key);
    assert.ok(tr.includes(`'${key}':`), 'missing TR key ' + key);
  }
});

// The dashboard checks the components by itself (owner decision, 12 Oct 2026),
// without wasting the host's time: one conditional scan when the stored one is
// missing or older than five minutes, only for a page on screen, never two at
// once, never more often than the window, and never again at once after a
// refusal. The mounted behaviour is in remote-state-mounted-batch3.test.mjs.
// Pano bileşenleri kendisi kontrol eder (sahip kararı, 12 Eki 2026), makinenin
// zamanını boşa harcamadan.
test('the dashboard checks the components by itself, conditionally, and only when it is seen', () => {
  // One window for the card and for what the check asks the server.
  assert.match(dashboard, /const SCAN_WINDOW_SECONDS = 300;/);
  assert.match(dashboard, /return now - effective <= SCAN_WINDOW_MS;/);
  assert.match(dashboard, /`\/api\/v1\/managed-services\/scan\?max_age_seconds=\$\{SCAN_WINDOW_SECONDS\}`/);
  // When it runs: the stored records were read, they are not fresh, the page
  // is visible, nothing is in flight, and the last automatic check is a
  // window ago.
  assert.match(dashboard,
    // A stored record the server cannot verify is due as a stale one is (2026-10-15).
    /const autoCheckDue = \(scanRead === 'unverified' \|\| \(scanRead === 'known' && !serviceScanFresh\)\)\s*&& visible\s*&& !componentScanBusy\s*&& \(lastAutoCheckAt === null \|\| freshnessNow - lastAutoCheckAt >= SCAN_WINDOW_MS\);/);
  assert.match(dashboard, /document\.visibilityState === 'visible'/);
  assert.match(dashboard, /document\.addEventListener\('visibilitychange', onVisibility\)/);
  assert.match(dashboard, /return \(\) => document\.removeEventListener\('visibilitychange', onVisibility\)/);
  // Never two at once: a ref, because two effects of one render share state.
  assert.match(dashboard, /if \(scanInFlight\.current\) return;\s*scanInFlight\.current = true;/);
  assert.match(dashboard, /finally \{\s*scanInFlight\.current = false;\s*setComponentScanBusy\(false\);/);
  // The window is counted from the attempt, whatever it came to.
  assert.match(dashboard, /if \(mode === 'auto'\) setLastAutoCheckAt\(Date\.now\(\)\);/);
  // No timer starts a check: the only intervals are the stats poll, the
  // readiness poll and the freshness clock, and none of them scans.
  for (const match of dashboard.matchAll(/setInterval\(([^;]*)/g)) {
    assert.doesNotMatch(match[1], /scan/i, 'a timer must not scan: ' + match[1]);
  }
  // A refusal because a change holds the host is its own state, not a failure.
  assert.match(dashboard, /const SCAN_HELD_CODES = new Set\(\['server_setup_busy', 'service_operation_busy'\]\);/);
  assert.match(dashboard, /response\.status === 409 && refusal\.code && SCAN_HELD_CODES\.has\(refusal\.code\)/);
  // The check nobody pressed raises no toast.
  assert.match(dashboard, /if \(mode === 'operator'\) showToast\('error', reason\);/);
});

test('the system-services card has distinct, honest states', () => {
  const card = dashboard.slice(dashboard.indexOf("{servicesCard !== 'known' ? ("), dashboard.indexOf("{servicesCard !== 'known' ? (") + 6000);
  // Checking is not "Unknown".
  assert.match(card, /servicesCard === 'checking' \? \([\s\S]{0,600}?<div className="mt-2 min-h-\[3\.75rem\]">\s*<Checking label=\{t\('dashboard\.systemServicesChecking'\)\}/);
  // Could not check names the reason and offers the check in place.
  assert.match(card, /t\('dashboard\.systemServicesCheckFailed', \{\s*reason:/);
  assert.match(card, /\{\(servicesCard === 'failed' \|\| servicesCard === 'unread'\) && \(\s*<button\s*type="button"\s*onClick=\{scanComponents\}/);
  // A change in progress offers nothing to press.
  assert.match(card, /servicesCard === 'held' \? 'dashboard\.systemServicesCheckHeld'/);
  assert.doesNotMatch(card, /servicesCard === 'held'[^\n]*&& \(/);
  // The old normal state sent the owner to another page; it is gone.
  assert.doesNotMatch(dashboard, /systemServicesScanStale/);
  assert.doesNotMatch(card, /rescan/i);
  for (const key of [
    'dashboard.systemServicesChecking',
    'dashboard.systemServicesCheckFailed',
    'dashboard.systemServicesCheckHeld',
    'dashboard.systemServicesNotRecent',
    'dashboard.systemServicesConnectionLost',
    'dashboard.systemServicesRecordsUnread',
    'dashboard.attentionNoneChecking',
    'dashboard.attentionNoneCheckFailed',
    'dashboard.attentionNoneCheckHeld',
  ]) {
    assert.ok(en.includes(`'${key}':`), 'missing EN key ' + key);
    assert.ok(tr.includes(`'${key}':`), 'missing TR key ' + key);
  }
  for (const catalogue of [en, tr]) {
    assert.match(catalogue, /'dashboard\.systemServicesCheckFailed': '[^']*\{reason\}'/);
  }
});

// 12 Oct 2026 (set14, Debian 13 and Ubuntu 24.04): the server's scanned_at ran
// 1-2 s ahead of the browser and the old rule rejected a stamp later than the
// browser's clock, so the card said "Unknown" for 20-29 s after a successful
// check. Freshness is now measured from the moment the page learned of the scan.
test('scan freshness is measured from when the page received the scan, not from two clocks', () => {
  const fn = dashboard.match(/function effectiveScanTime[\s\S]*?\n}\n\nfunction freshScanTimestamp[\s\S]*?\n}\n/);
  assert.ok(fn, 'effectiveScanTime / freshScanTimestamp not found');
  assert.match(fn[0], /Math\.min\(scannedAt, observedAt\)/);
  assert.match(fn[0], /now - effective <= SCAN_WINDOW_MS/);
  assert.match(fn[0], /if \(!value\) return null;/);
  assert.match(fn[0], /!Number\.isFinite\(scannedAt\)\) return null/);
  assert.doesNotMatch(fn[0], /scannedAt > now/, 'a stamp later than the browser clock is rejected again');
  assert.doesNotMatch(dashboard, /scannedAt > now/);
  // Both places that learn of a scan record when they did, with the age the
  // server reported.
  assert.equal((dashboard.match(/setServiceScan\(\{ scannedAt: snapshot\.scannedAt, scanAgeSeconds: snapshot\.scanAgeSeconds, observedAt \}\)/g) || []).length, 2);
  assert.doesNotMatch(dashboard, /setServiceScannedAt/);
  assert.match(dashboard, /const SCAN_WINDOW_SECONDS = 300;/);
});

// 13 Oct 2026, the limit e6de64a5b left open: a server clock more than five
// minutes behind the browser's still read a just-finished scan as stale. The
// server now reports the scan's age on its own clock (scan_age_seconds) and the
// page subtracts it from when it received the answer; without the field the
// earlier rule stands.
test('the scan is dated by the age the server reports, with the earlier rule as the fallback', () => {
  const fn = dashboard.match(/function effectiveScanTime[\s\S]*?\n}\n\nfunction freshScanTimestamp[\s\S]*?\n}\n/);
  assert.ok(fn, 'effectiveScanTime / freshScanTimestamp not found');
  assert.match(fn[0], /if \(ageSeconds !== null\) \{\s*if \(!Number\.isFinite\(ageSeconds\) \|\| ageSeconds < 0\) return null;\s*return observedAt - ageSeconds \* 1000;\s*\}/);
  // The age wins over the stamp; the stamp rule stays for an older server.
  assert.ok(fn[0].indexOf('observedAt - ageSeconds * 1000') < fn[0].indexOf('Math.min(scannedAt, observedAt)'));
  assert.match(fn[0], /Math\.min\(scannedAt, observedAt\)/);
  // The decoder accepts the optional field and still fails a wrong one closed.
  const decoder = dashboard.match(/function decodeDashboardServices[\s\S]*?\n}\n/);
  assert.ok(decoder, 'decodeDashboardServices not found');
  assert.match(decoder[0], /const scanAge = payload\.scan_age_seconds;/);
  assert.match(decoder[0], /scanAge !== undefined\s*&& scanAge !== null\s*&& \(typeof scanAge !== 'number' \|\| !Number\.isFinite\(scanAge\) \|\| scannedAt === null\)/);
  assert.match(decoder[0], /scanAgeSeconds: typeof scanAge === 'number' \? scanAge : null,/);
});

// set14 O1: with the Agent stopped the card said "The server hit an internal
// error". The Panel now answers 503 AGENT_UNREACHABLE and the card's reason is
// that code's sentence, in both languages.
test('an unreachable Agent is named on the card in EN and TR', () => {
  const httperr = readFileSync(new URL('../../cmd/panel/httperr.go', import.meta.url), 'utf8');
  assert.match(httperr, /errCodeAgentUnreachable = "AGENT_UNREACHABLE"/);
  assert.match(httperr, /writeCodedError\(w, http\.StatusServiceUnavailable, errCodeAgentUnreachable, agentUnreachableMessage, ""\)/);
  for (const [catalogue, words] of [
    [en, ['not reachable', 'nothing was changed', 'server owner', 'sudo systemctl status celikpanel-agent', 'try again']],
    [tr, ['ulaşılamıyor', 'hiçbir şey değiştirilmedi', 'Sunucunun sahibi', 'sudo systemctl status celikpanel-agent', 'yeniden deneyin']],
  ]) {
    const line = catalogue.match(/'err\.AGENT_UNREACHABLE': '([^']*)'/);
    assert.ok(line, 'missing err.AGENT_UNREACHABLE');
    for (const word of words) assert.ok(line[1].includes(word), `${word} missing from ${line[1]}`);
    assert.doesNotMatch(line[1], /internal error|iç hata/);
  }
  // The card's failure reason is the coded answer's sentence (err.<CODE>).
  assert.match(dashboard, /fail\(apiErrorText\(refusal, t, 'services\.scanFailed'\)\);/);
});
