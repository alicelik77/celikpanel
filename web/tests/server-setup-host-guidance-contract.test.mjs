import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';

const setup = readFileSync(new URL('../src/components/ServerSetup.tsx', import.meta.url), 'utf8');
const en = englishScreens;
const tr = turkishScreens;

const mapped = (code) => {
  const match = setup.match(new RegExp(String.raw`\n\s*${code}: '([^']+)'`));
  return match ? match[1] : null;
};

// Decision B/C (2026-09-30): a rolled-back DNS step and a firewall that cannot
// be checked each get their own text; neither falls back to the nameserver
// identity or an internal error.
test('setup maps rolled-back DNS and firewall host conditions to their own guidance', () => {
  const expected = {
    server_setup_dns_rolled_back: 'setup.guide.dnsRolledBack',
    host_restart_required: 'setup.blocker.hostRestart',
    firewall_kernel_unavailable: 'setup.blocker.firewallKernel',
    firewall_engine_unavailable: 'setup.blocker.firewallEngine',
    firewall_busy: 'setup.blocker.firewallBusy',
    firewall_status_unknown: 'setup.blocker.firewallUnknown',
  };
  for (const [code, key] of Object.entries(expected)) {
    assert.equal(mapped(code), key, code);
    assert.notEqual(mapped(code), 'setup.blocker.dnsIdentity', code);
    assert.match(en, new RegExp(`"${key.replaceAll('.', String.raw`\.`)}": "`), `EN ${key}`);
    assert.match(tr, new RegExp(`"${key.replaceAll('.', String.raw`\.`)}": "`), `TR ${key}`);
  }
  // Beta-1 re-run P6: a verified failure with both next actions and no restart.
  assert.match(en, /"setup\.guide\.dnsRolledBack": "The DNS engine installation did not complete and was undone: no DNS engine is running, and any packages it installed were kept stopped\. Nothing else was changed, and no restart is needed\. You, the server administrator: choose Review a revised plan, then keep this DNS software to try again or choose the other one; setup continues from the new plan\."/);
  assert.match(tr, /"setup\.guide\.dnsRolledBack": "DNS motoru kurulumu tamamlanmadı ve geri alındı: çalışan bir DNS motoru yok, kurduğu paketler varsa durdurulmuş olarak saklandı\. Başka hiçbir şey değiştirilmedi ve yeniden başlatma gerekmez\. Siz, sunucu yöneticisi: Düzeltilmiş planı incele'yi seçin; bu DNS yazılımıyla yeniden deneyin ya da diğerini seçin; kurulum yeni plandan sürer\."/);
  assert.match(en, /"setup\.blocker\.hostRestart": "This server was updated and must be restarted before its firewall can be checked\. Restart the server, then open setup again; your draft continues and nothing was changed\."/);
});

// Arch PowerDNS drill apd-e (2026-10-30): when the Agent keeps the switch
// journal, the step is never shown as undone, and a new install that the
// journal would refuse waits with its own reason instead of "running".
test('setup never calls a kept-journal DNS operation undone and explains a blocked retry', () => {
  const textOf = (catalogue, key) => {
    const match = catalogue.match(new RegExp(`"${key.replaceAll('.', String.raw`\.`)}": "([^"]+)"`));
    assert.ok(match, key);
    return match[1];
  };
  assert.equal(mapped('server_setup_dns_recovery_held'), 'setup.guide.dnsRecoveryHeld');
  assert.equal(mapped('server_setup_dns_journal_retained'), 'setup.guide.dnsJournalRetained');
  const heldEN = textOf(en, 'setup.guide.dnsRecoveryHeld');
  const heldTR = textOf(tr, 'setup.guide.dnsRecoveryHeld');
  assert.doesNotMatch(heldEN, /complete and was undone|\bundid\b/);
  assert.match(heldEN, /not shown as undone/);
  assert.match(heldEN, /You, the server administrator: run the status command shown under Technical details/);
  assert.match(heldEN, /checks this same operation again automatically/);
  assert.doesNotMatch(heldTR, /ve geri alındı:/);
  assert.match(heldTR, /geri alındı denmiyor/);
  assert.match(heldTR, /Siz, sunucu yöneticisi: Teknik ayrıntılar altında gösterilen durum komutunu/);
  const retainedEN = textOf(en, 'setup.guide.dnsJournalRetained');
  const retainedTR = textOf(tr, 'setup.guide.dnsJournalRetained');
  assert.match(retainedEN, /^Setup has not started the DNS installation/);
  assert.match(retainedEN, /Nothing was started\./);
  assert.match(retainedEN, /starts the installation once that record is resolved/);
  assert.match(retainedTR, /^Kurulum DNS kurulumunu başlatmadı/);
  assert.match(retainedTR, /Hiçbir şey başlatılmadı\./);
  assert.match(retainedTR, /kayıt çözülünce kurulumu başlatır/);
});
