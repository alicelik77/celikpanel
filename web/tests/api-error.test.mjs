import assert from 'node:assert/strict';
import test from 'node:test';

import { apiErrorText, readApiError } from '../src/lib/apiError.ts';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

// One catalogue per locale, assembled from the files it now ships in (register
// R-060; the screen half is two files). What the running panel looks a key up
// in is this union.
// Dil basina tek katalog, artik geldigi iki yaridan birlestirilir (R-060).
const en = { ...enShell, ...enScreens, ...enServerScreens };
const tr = { ...trShell, ...trScreens, ...trServerScreens };

test('readApiError keeps only string detail lines from JSON errors', async () => {
  const response = new Response(JSON.stringify({
    error: 'installation failed',
    code: 'service_install_failed',
    partial_success: true,
    mutation_applied: true,
    details: ['postgresql@18-main is inactive', null, 18, {}, '+2'],
  }), {
    status: 500,
    headers: { 'content-type': 'application/json' },
  });

  const error = await readApiError(response);

  assert.equal(error.message, 'installation failed');
  assert.equal(error.code, 'service_install_failed');
  assert.equal(error.partialSuccess, true);
  assert.equal(error.mutationApplied, true);
  assert.deepEqual(error.details, ['postgresql@18-main is inactive', '+2']);
});

test('readApiError ignores a non-array details value', async () => {
  const response = new Response(JSON.stringify({
    error: 'installation failed',
    partial_success: 'true',
    mutation_applied: false,
    details: 'not an array',
  }), { status: 500 });

  const error = await readApiError(response);

  assert.equal(error.details, undefined);
  assert.equal(error.partialSuccess, undefined);
  assert.equal(error.mutationApplied, undefined);
});

test('DNS engine outcome codes replace remote text in EN and TR', () => {
  const translate = (catalog) => (key) => catalog[key] ?? key;
  for (const code of [
    'DNS_ENGINE_PLAN_REJECTED',
    'DNS_ENGINE_CHANGE_NOT_COMMITTED',
    'DNS_ENGINE_STATE_UNVERIFIED',
    'DNS_ENGINE_CHANGE_APPLIED_REFRESH_REQUIRED',
  ]) {
    const remoteText = '/etc/bind/private token=do-not-leak';
    const english = apiErrorText({ code, message: remoteText }, translate(en));
    const turkish = apiErrorText({ code, message: remoteText }, translate(tr));
    assert.equal(english, en['err.' + code]);
    assert.equal(turkish, tr['err.' + code]);
    assert.ok(!english.includes(remoteText) && !turkish.includes(remoteText));
  }
});

test('DNS engine not-committed copy does not overclaim the target serving state', () => {
  assert.equal(
    en['err.DNS_ENGINE_CHANGE_NOT_COMMITTED'],
    'The DNS engine change was not committed. The pre-operation serving state was verified; packages or setup files may still have changed. Refresh state before creating a new review.',
  );
  assert.equal(
    tr['err.DNS_ENGINE_CHANGE_NOT_COMMITTED'],
    'DNS motoru değişikliği kesinleştirilmedi. İşlem öncesindeki hizmet durumu doğrulandı; paketler veya kurulum dosyaları yine de değişmiş olabilir. Yeni bir inceleme oluşturmadan önce DNS durumunu yenileyin.',
  );
  assert.doesNotMatch(en['err.DNS_ENGINE_CHANGE_NOT_COMMITTED'], /not serving|reverted/i);
  assert.doesNotMatch(tr['err.DNS_ENGINE_CHANGE_NOT_COMMITTED'], /hizmet vermiyor|geri alınd/i);
});

test('DNS engine plan-rejected copy is mode-neutral', () => {
  assert.equal(
    en['err.DNS_ENGINE_PLAN_REJECTED'],
    'The server agent rejected the reviewed DNS plan. The DNS engine change was not committed. Verify current DNS state before creating a new review; do not retry the old confirmation.',
  );
  assert.equal(
    tr['err.DNS_ENGINE_PLAN_REJECTED'],
    'Sunucu agentı incelenen DNS planını reddetti. DNS motoru değişikliği kesinleştirilmedi. Yeni bir inceleme oluşturmadan önce güncel DNS durumunu doğrulayın; eski onayı yeniden denemeyin.',
  );
  assert.doesNotMatch(en['err.DNS_ENGINE_PLAN_REJECTED'], /activation|not serving|stopped/i);
  assert.doesNotMatch(tr['err.DNS_ENGINE_PLAN_REJECTED'], /etkinleştirme|hizmet vermiyor|durdur/i);
});

test('platform refusal codes have localized operator-safe text', () => {
  const translate = (catalog) => (key) => catalog[key] ?? key;
  for (const code of [
    'PLATFORM_CAPABILITY_UNAVAILABLE',
    'PLATFORM_IDENTITY_UNAVAILABLE',
  ]) {
    const remoteText = 'remote secret and command output';
    const english = apiErrorText({ code, message: remoteText }, translate(en));
    const turkish = apiErrorText({ code, message: remoteText }, translate(tr));
    assert.equal(english, en['err.' + code]);
    assert.equal(turkish, tr['err.' + code]);
    assert.ok(english.length > 0 && turkish.length > 0);
    assert.ok(!english.includes(remoteText) && !turkish.includes(remoteText));
    assert.ok(!english.includes('No host changes were made'));
    assert.ok(!turkish.includes('hiçbir değişiklik yapılmadı'));
  }
});

test('mail profile hostname refusal replaces untrusted transport text in EN and TR', () => {
  const translate = (catalog) => (key) => catalog[key] ?? key;
  const code = 'mail_profile_server_hostname_invalid';
  const remoteText = '/etc/private: hostnamectl failed with secret command output';

  const english = apiErrorText({ code, message: remoteText }, translate(en));
  const turkish = apiErrorText({ code, message: remoteText }, translate(tr));

  assert.equal(english, en['err.' + code]);
  assert.equal(turkish, tr['err.' + code]);
  assert.match(english, /fully qualified domain name/);
  assert.match(turkish, /tam nitelikli bir alan adı/);
  assert.ok(!english.includes(remoteText) && !turkish.includes(remoteText));
});

// R-036. The refusal that used to be a dead end now names a field to fill, and
// it too replaces whatever the remote text was.
test('mail profile missing-hostname refusal names the field in EN and TR', () => {
  const translate = (catalog) => (key) => catalog[key] ?? key;
  const code = 'mail_profile_server_hostname_required';
  const remoteText = '/etc/private: hostname read failed with secret command output';

  const english = apiErrorText({ code, message: remoteText }, translate(en));
  const turkish = apiErrorText({ code, message: remoteText }, translate(tr));

  assert.equal(english, en['err.' + code]);
  assert.equal(turkish, tr['err.' + code]);
  assert.match(english, /mail\.example\.com/);
  assert.match(turkish, /mail\.example\.com/);
  assert.ok(!english.includes(remoteText) && !turkish.includes(remoteText));
});

// R-035. Each of the three SSH outcomes is its own translated sentence, so the
// browser never shows the agent's own English back to the operator.
test('firewall SSH refusals are three separate translated sentences', () => {
  const translate = (catalog) => (key) => catalog[key] ?? key;
  const remoteText = 'ss permission denied: /usr/sbin/ss exited 1';
  const codes = [
    'FIREWALL_NO_SSH_SERVICE',
    'FIREWALL_SSH_NOT_LISTENING',
    'FIREWALL_SSH_DISCOVERY_FAILED',
  ];
  const seen = new Set();
  for (const code of codes) {
    const english = apiErrorText({ code, message: remoteText }, translate(en));
    const turkish = apiErrorText({ code, message: remoteText }, translate(tr));
    assert.equal(english, en['err.' + code]);
    assert.equal(turkish, tr['err.' + code]);
    assert.ok(english !== 'err.' + code && turkish !== 'err.' + code);
    assert.ok(!english.includes(remoteText) && !turkish.includes(remoteText));
    assert.ok(!seen.has(english), `${code} reuses another refusal's sentence`);
    seen.add(english);
  }
});

// A site file whose service refused CelikPanel's text says whether the previous
// file is back (2026-10-11): `restore: unconfirmed` selects its own sentence;
// confirmed (or absent, from an older panel) keeps the sentence that says the
// file was put back.
test('a service refusal whose restore is not confirmed never says the file was put back', async () => {
  const body = (restore) => new Response(JSON.stringify({
    error: 'server words', code: 'SITE_CONFIG_SERVICE_REFUSED', reason: 'php_pool', detail: 'reload_failed', restore,
    details: ['php-fpm reload: exit status 1; rollback incomplete: restore: read-only file system'],
  }), { status: 502, headers: { 'content-type': 'application/json' } });
  for (const [catalogue, notConfirmed, putBack] of [
    [en, 'whether it is in place is not confirmed', 'so the previous pool file was put back'],
    [tr, 'yerinde olup olmadığı doğrulanmadı', 'bu yüzden önceki havuz dosyası geri kondu'],
  ]) {
    const t = (key) => catalogue[key] ?? key;
    const unconfirmed = await readApiError(body('unconfirmed'));
    assert.equal(unconfirmed.restore, 'unconfirmed');
    const said = apiErrorText(unconfirmed, t);
    assert.ok(said.includes(notConfirmed) && !said.includes(putBack), said);
    const confirmed = apiErrorText(await readApiError(body('confirmed')), t);
    assert.ok(confirmed.includes(putBack) && !confirmed.includes(notConfirmed), confirmed);
    const older = apiErrorText(await readApiError(body(undefined)), t);
    assert.ok(older.includes(putBack), older);
  }
  // nginx's refusal has no sentence per reason: the code's unconfirmed one.
  const nginx = await readApiError(new Response(JSON.stringify({
    error: 'x', code: 'SITE_CONFIG_NGINX_REFUSED', reason: 'nginx_refused', restore: 'unconfirmed',
  }), { status: 502, headers: { 'content-type': 'application/json' } }));
  assert.equal(apiErrorText(nginx, (key) => en[key] ?? key), en['err.SITE_CONFIG_NGINX_REFUSED.unconfirmed']);
});

// A put-back after which the service was not reloaded (alpha.83): the sentence
// says how work resumes, and a service that is not running now (service_active
// false) never "keeps running" - it gets its own sentence, with the check's
// line only when the answer carries one.
test('a not-reloaded put-back says how to resume and never says a stopped service keeps running', async () => {
  const answer = async (reason, serviceReason, serviceActive, vars) => readApiError(new Response(JSON.stringify({
    error: 'server words', code: 'SITE_CONFIG_SERVICE_REFUSED', reason, restore: 'confirmed', service: 'not_reloaded',
    service_reason: serviceReason, service_active: serviceActive, vars,
  }), { status: 502, headers: { 'content-type': 'application/json' } }));
  const line = 'zz-owner.conf:1 unknown entry';
  for (const [catalogue, resume, running, notRunning] of [
    [en, 'choose again if you still want CelikPanel’s text', 'keeps running', 'is not running now'],
    [tr, 'CelikPanel’in metnini hâlâ istiyorsanız yeniden seçin', 'çalışmaya devam ediyor', 'şu anda çalışmıyor'],
  ]) {
    const t = (key, vars) => {
      const text = catalogue[key];
      if (text === undefined) return key;
      return vars ? text.replace(/\{(\w+)\}/g, (m, name) => vars[name] ?? m) : text;
    };
    for (const kind of ['php_pool', 'nginx_vhost', 'app_unit']) {
      const reason = kind === 'app_unit' ? 'check_unavailable' : 'config_refused_elsewhere';
      // Running (or unknown): the sentence that says it keeps running, with the resume.
      for (const active of ['true', 'unknown', undefined]) {
        const said = apiErrorText(await answer(kind, reason, active, { line }), t);
        assert.ok(said.includes(resume) && said.includes(running), `${kind}/${active}: ${said}`);
        assert.ok(!said.includes(notRunning), said);
      }
      // Not running: its own sentence, never "keeps running".
      for (const serviceReason of [reason, 'config_refused_elsewhere', undefined]) {
        for (const vars of [{ line }, undefined]) {
          const said = apiErrorText(await answer(kind, serviceReason, 'false', vars), t);
          assert.ok(said.includes(notRunning) && said.includes(resume), `${kind}/${serviceReason}/${!!vars}: ${said}`);
          assert.ok(!said.includes(running) && !said.includes('{line}'), said);
          if (vars && serviceReason !== 'check_unavailable') assert.ok(said.includes(line), said);
          if (!vars) assert.ok(!said.includes(line), said);
        }
      }
    }
    // The check that could not be run says so and leaves the restart to the administrator.
    const unavailable = apiErrorText(await answer('app_unit', 'check_unavailable', 'true', undefined), t);
    assert.ok(unavailable.includes(catalogue === en ? 'could not be run' : 'çalıştırılamadı'), unavailable);
  }
});
