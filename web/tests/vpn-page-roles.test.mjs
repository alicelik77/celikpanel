import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const vpn = readFileSync(new URL('../src/components/VPNPage.tsx', import.meta.url), 'utf8');
const api = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8');

test('a non-administrator the server does not offer VPN to gets one plain sentence, in both languages', () => {
  assert.match(vpn, /if \(role !== 'admin' && user\?\.features\?\.vpn !== true\) \{\s+return <VPNNotOffered \/>;/);
  assert.match(vpn, /if \(status && !status\.installed && !isAdmin\) \{\s+return <VPNNotOffered \/>;/);
  assert.match(englishCatalogue, /'vpn\.notOffered': 'VPN is not offered on this server\. Ask your provider if you need it\.'/);
  assert.match(turkishCatalogue, /'vpn\.notOffered': 'Bu sunucuda VPN sunulmuyor\. İhtiyacınız varsa sağlayıcınıza başvurun\.'/);
});

test('the synchronization box and its ledger value never show before WireGuard is installed, and only to an administrator', () => {
  assert.match(vpn, /\{isAdmin && status\.installed && \(\s+<ReadOnlyValue\s+label=\{t\('vpn\.desiredLedger'\)\}/);
  assert.match(vpn, /\{isAdmin && status\.installed && \(\s+<section className=\{`rounded-xl border p-5 \$\{/);
});

test('jargon, the release badge, and the policy and key cards are for the administrator only', () => {
  assert.match(vpn, /\{isAdmin && <StateBadge tone="neutral">\{t\('vpn\.releaseManaged'\)\}<\/StateBadge>\}/);
  assert.match(vpn, /filter\(\(\[id\]\) => isAdmin \|\| id !== 'policy'\)/);
  assert.match(vpn, /\{isAdmin && \(\s+<section className="rounded-xl border border-border bg-surface p-5">\s+<Server className="h-5 w-5 text-primary" \/>\s+<h3 className="mt-3 font-semibold text-fg">\{t\('vpn\.serverPolicyCard'\)\}/);
  assert.match(vpn, /\{isAdmin && \(\s+<section className="rounded-xl border border-border bg-surface p-5">\s+<KeyRound/);
  assert.doesNotMatch(vpn, /vpn\.adminInstallRequired/);
});

test('the identity decoder passes the vpn flag only when it is exactly true', () => {
  assert.match(api, /vpn: rawFeatures\.vpn === true,/);
});

test('the new strings exist in English and Turkish', () => {
  for (const key of ['vpn.notOffered', 'vpn.colState']) {
    assert.ok(englishCatalogue.includes(`'${key}'`), `en ${key}`);
    assert.ok(turkishCatalogue.includes(`'${key}'`), `tr ${key}`);
  }
});
