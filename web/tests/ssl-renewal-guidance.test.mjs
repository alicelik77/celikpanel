import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';

// Who renews a website certificate and what the administrator does (D-039,
// D-024): a certificate the operating system's certbot renews names its
// reason, then the concrete next action, then how it resumes; one an earlier
// release issued says that only the running Panel renews it and how to move
// it; unknown is never a problem.
const source = readFileSync(new URL('../src/lib/sslRenewal.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const { renewalGuidance, renewalLabelKey, isRenewalProblem, nativeRenewalState } = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'));

const native = (state, extra = {}) => ({
  type: 'letsencrypt', renewal_status: '', renewal_owner: 'operating_system',
  renewal: { state, lineage_name: 'celikpanel-site-42-00112233445566778899aabb', scheduler: { name: 'certbot.timer', enabled: true, active: true, known: true }, ...extra },
});

const states = ['scheduled', 'renewed', 'failed', 'overdue', 'expired', 'scheduler_missing', 'disabled', 'deploy_incomplete',
  'hook_missing', 'hook_changed', 'owner_configured', 'lineage_missing', 'unknown'];

test('every OS renewal state has a label, a reason and, when it needs someone, an action, in both languages', () => {
  for (const state of states) {
    const cert = native(state, { last_failed: { at: '2026-10-30T00:00:00Z', detail: 'connection refused' } });
    const label = renewalLabelKey(cert);
    const guide = renewalGuidance(cert);
    assert.equal(label, `ssl.renewal.os.${state}`);
    assert.equal(guide.reasonKey, `ssl.renewalGuide.${state === 'renewed' ? 'renewed' : state}`);
    assert.equal(Boolean(guide.actionKey), isRenewalProblem(state), state);
    for (const [locale, catalogue] of [['en', englishScreens], ['tr', turkishScreens]]) {
      assert.ok(catalogue.includes(`'${label}':`), `${locale} ${label}`);
      if (guide.tone !== 'ok') assert.ok(catalogue.includes(`'${guide.reasonKey}':`), `${locale} ${guide.reasonKey}`);
      if (guide.actionKey) assert.ok(catalogue.includes(`'${guide.actionKey}':`), `${locale} ${guide.actionKey}`);
    }
  }
  assert.equal(renewalGuidance(native('failed', { last_failed: { at: '', detail: 'connection refused' } })).params.detail, 'connection refused');
  assert.equal(renewalGuidance(native('overdue', { renew_after: '2026-10-01T00:00:00Z' })).params.date, '2026-10-01');
});

test('unknown is not a problem and an unreadable state concludes nothing', () => {
  assert.equal(isRenewalProblem('unknown'), false);
  const guide = renewalGuidance({ type: 'letsencrypt', renewal_status: 'os_failed', renewal_owner: 'operating_system', renewal_read_error: 'x' });
  // The last recorded state is still shown, not replaced by "unknown".
  assert.equal(nativeRenewalState({ type: 'letsencrypt', renewal_status: 'os_failed', renewal_owner: 'operating_system' }), 'failed');
  assert.equal(guide.reasonKey, 'ssl.renewalGuide.failed');
  const unread = renewalGuidance({ type: 'letsencrypt', renewal_status: '', renewal_owner: 'operating_system', renewal_read_error: 'x' });
  assert.equal(unread.reasonKey, 'ssl.renewalGuide.unknown');
  assert.equal(unread.actionKey, undefined);
});

test('a certificate of an earlier release says only the running Panel renews it and how to move it', () => {
  const guide = renewalGuidance({ type: 'letsencrypt', renewal_status: 'renewed', renewal_owner: 'panel' });
  assert.equal(guide.reasonKey, 'ssl.renewalGuide.panel');
  assert.equal(guide.actionKey, 'ssl.renewalGuide.panel.action');
  assert.equal(renewalLabelKey({ type: 'letsencrypt', renewal_status: 'renewed', renewal_owner: 'panel' }), null);
  assert.equal(renewalGuidance({ type: 'custom', renewal_status: '' }), null);
});

test('the next actions are concrete and name the server administrator', () => {
  const sentence = (catalogue, key) => {
    const match = catalogue.match(new RegExp(`'${key.replace(/\./g, '\\.')}': '([^']*)'`));
    assert.ok(match, key);
    return match[1];
  };
  assert.match(sentence(englishScreens, 'ssl.renewalGuide.who'), /server administrator/);
  assert.match(sentence(turkishScreens, 'ssl.renewalGuide.who'), /sunucu yöneticisi/);
  for (const catalogue of [englishScreens, turkishScreens]) {
    assert.match(sentence(catalogue, 'ssl.renewalGuide.failed.action'), /sudo certbot renew --cert-name \{lineage\}/);
    assert.match(sentence(catalogue, 'ssl.renewalGuide.scheduler_missing.action'), /systemctl enable --now certbot\.timer/);
    assert.match(sentence(catalogue, 'ssl.renewalGuide.deploy_incomplete.action'), /systemctl reload nginx/);
    assert.match(sentence(catalogue, 'dashboard.certRenewalProblem'), /\{domain\}/);
  }
});
