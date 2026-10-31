import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';
import { englishCatalogue, enScreens, trScreens, turkishCatalogue } from './locale-catalogue.mjs';

// alpha.114, the owner's two rules of 31 Oct 2026 on the screens this package
// touched: (a) no change needs a page refresh, (b) the person always sees what
// happened and what to do: no unexplained redirect, no dead end, no internal
// wording. The mounted cases (Domains create/delete rows, bulk confirmation,
// databases by role) are in remote-state-mounted.test.mjs; the mechanism in
// remote-invalidation.test.mjs.
// alpha.114: sahibin iki kurali bu paketin ekranlarinda.
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const value = (catalogue, key) => {
  const match = catalogue.match(new RegExp(`'${key.replace(/\./g, '\\.')}':\\s*'((?:[^'\\\\]|\\\\.)*)'`));
  return match ? match[1] : undefined;
};

test('Add domain tells every reader of domains, and a partial success opens a page that says what needs attention', () => {
  const modal = source('components/AddDomainModal.tsx');
  assert.ok((modal.match(/invalidate\('domains', 'dns'\)/g) ?? []).length >= 2, 'create and partial success both invalidate');
  assert.match(modal, /navigate\(withAttention\(action\)\)/);
  assert.match(modal, /attention=dns-publication/);
  const detail = source('components/DomainDetail.tsx');
  assert.match(detail, /searchParams\.get\('attention'\) === 'dns-publication'/);
  for (const key of ['dnsTitle', 'dnsWhy', 'dnsWho', 'dnsNext', 'dnsThen']) {
    assert.ok(enScreens[`domains.attention.${key}`] && trScreens[`domains.attention.${key}`], key);
  }
});

test('Domains: a delete tells every reader; the kept-records card is shown after a create', () => {
  const domains = source('components/Domains.tsx');
  assert.match(domains, /invalidate\('domains', 'dns'\)/);
  assert.match(domains, /<KeptSiteRecords/);
  assert.doesNotMatch(domains, /names\.forEach\(\(d\) => handleDelete/, 'the bulk delete still asks once per domain');
  assert.match(enScreens['domains.confirmBulkDelete'], /\{n\}.*\{names\}/s);
  assert.match(trScreens['domains.confirmBulkDelete'], /\{n\}.*\{names\}/s);
  assert.match(source('components/InfrastructureZone.tsx'), /initialDomain=\{zone\}/);
});

test('install buttons lead to the right place, in the panel, without a page load', () => {
  const vpn = source('components/VPNPage.tsx');
  assert.doesNotMatch(vpn, /window\.location\.assign\('\/services'\)/);
  assert.match(vpn, /navigate\('\/services\?component=wireguard'\)/);
  const list = source('components/ServiceList.tsx');
  assert.match(list, /get\('component'\)/, 'the Components page does not open at one component');
  assert.match(list, /if \(linkedComponent\) return false;/, 'a linked component that is not installed would stay hidden');
  // The DNS engine is chosen on the DNS page's own engine card: no ping-pong.
  const dns = source('components/DNSServerSettings.tsx');
  assert.doesNotMatch(dns, /to="\/services"/);
  assert.match(dns, /onClick=\{focusEngineCard\}/);
  assert.match(dns, /id=\{ENGINE_CARD_ID\}/);
  // PowerDNS: records are managed from the DNS page; a failed repair says why.
  const pdns = source('components/PowerDNSManagement.tsx');
  assert.match(pdns, /navigate\('\/dns-infrastructure'\)/);
  assert.match(pdns, /readApiError\(res\)/);
  assert.match(pdns, /pdns\.repairFailedNext/);
  // The Dashboard's mail card opens mailboxes, not a bare list.
  const dashboard = source('components/Dashboard.tsx');
  assert.match(dashboard, /to=\{mailCardTarget\(domains\)\}/);
  assert.match(dashboard, /\?tab=mail/);
  assert.match(source('components/DomainDetail.tsx'), /requestedTab === 'mail'/);
});

test('the webmail cleanup link always ends in a dialog that says why, or a sentence', () => {
  const list = source('components/ServiceList.tsx');
  assert.match(list, /showToast\('info', t\('services\.cleanupWebmailNotNeeded'\)\)/);
  assert.match(list, /data-webmail-cleanup-why/);
  assert.match(list, /data-builtin-sftp/, 'the built-in SFTP explanation is still only a tooltip');
  for (const key of ['services.cleanupWebmailWhy', 'services.cleanupWebmailNotNeeded', 'services.useBuiltInSFTPHint']) {
    assert.ok(enScreens[key] && trScreens[key], key);
  }
  assert.doesNotMatch(enScreens['services.useBuiltInSFTPHint'], /still being completed/);
});

test('a complete import offers the site, its DNS and its mailboxes; the import tells every reader', () => {
  const page = source('components/ImportPage.tsx');
  assert.match(page, /data-import-next/);
  assert.match(page, /\?tab=dns/);
  assert.match(page, /\?tab=mail/);
  assert.match(page, /invalidate\('domains', 'dns', 'mail', 'databases'\)/);
});

test('no internal wording in the rewritten sentences, EN and TR', () => {
  const internal = /operation id|operation identity|recovery marker|pre-request|terminal|session storage|structurally|out of band|supported marker|işaretçi|terminal durumda|oturum depolaması|ayrı kanaldan doğrulayın|desteklenen işaretle/i;
  for (const [catalogue, keys] of [
    [englishCatalogue, ['panelUpdate.identityMismatchCleared', 'services.operation.backgroundHint', 'services.operation.recoveryStorageUnavailable', 'securityAudit.code.rebootStateUnknown', 'securityAudit.code.signedUpdateIdentityUnverified']],
    [turkishCatalogue, ['panelUpdate.identityMismatchCleared', 'services.operation.backgroundHint', 'services.operation.recoveryStorageUnavailable', 'securityAudit.code.rebootStateUnknown', 'securityAudit.code.signedUpdateIdentityUnverified']],
  ]) {
    for (const key of keys) {
      const text = value(catalogue, key);
      assert.ok(text, key);
      assert.doesNotMatch(text, internal, key);
    }
  }
  assert.match(value(englishCatalogue, 'services.operation.recoveryStorageUnavailable'), /Allow this site to store data in your browser, then try again\./);
});

test('the dead StartGuide is gone', () => {
  assert.equal(existsSync(new URL('../src/components/StartGuide.tsx', import.meta.url)), false);
});
