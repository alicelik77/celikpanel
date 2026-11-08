// alpha.130 ops lane: the update sources in plain words, the SSH password
// sign-in card's decisions and words, the Problems list's "stale" reason and
// the security audit's certificate names.
//
// alpha.130: güncelleme kaynakları düz dille, SSH parola kartı, eski okuma.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

const load = async (path) => {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
  return import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'));
};
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

const { describeOrigin, describeOrigins } = await load('../src/lib/updateOrigins.ts');
const { enSSHPassword } = await load('../src/i18n/sshPassword/en.ts');

test('Debian and Ubuntu patterns read as who publishes them and what they carry', () => {
  assert.deepEqual(describeOrigin('origin=Debian,codename=${distro_codename}-security,label=Debian-Security'), { origin: 'Debian', kind: 'security' });
  assert.deepEqual(describeOrigin('origin=Debian,codename=${distro_codename},label=Debian'), { origin: 'Debian', kind: 'release' });
  assert.deepEqual(describeOrigin('origin=Debian,codename=${distro_codename}-updates'), { origin: 'Debian', kind: 'updates' });
  assert.deepEqual(describeOrigin('o=Debian Backports,n=${distro_codename}-backports'), { origin: 'Debian Backports', kind: 'vendor' });
  assert.deepEqual(describeOrigin('${distro_id}:${distro_codename}-security'), { origin: null, kind: 'security' });
  assert.deepEqual(describeOrigin('${distro_id}ESMApps:${distro_codename}-apps-security'), { origin: null, kind: 'esm' });
  assert.deepEqual(describeOrigin('Ubuntu:noble-updates'), { origin: 'Ubuntu', kind: 'updates' });
  assert.deepEqual(describeOrigin('origin=PostgreSQL Global Development Group'), { origin: 'PostgreSQL Global Development Group', kind: 'vendor' });
  assert.deepEqual(describeOrigin('nonsense'), { origin: null, kind: 'other' });
});

test('each source is said once, security first, and ${distro_id} takes the named distribution', () => {
  const debian = describeOrigins([
    'origin=Debian,codename=${distro_codename},label=Debian',
    'origin=Debian,codename=${distro_codename}-security,label=Debian-Security',
    'origin=Debian,codename=${distro_codename}-security,label=Debian-Security',
  ]);
  assert.deepEqual(debian, [{ origin: 'Debian', kind: 'security' }, { origin: 'Debian', kind: 'release' }]);
  const ubuntu = describeOrigins(['${distro_id}:${distro_codename}', '${distro_id}:${distro_codename}-security', 'Ubuntu:noble-security']);
  assert.deepEqual(ubuntu[0], { origin: 'Ubuntu', kind: 'security' });
});

test('the card shows plain sources and keeps the raw patterns behind a disclosure', () => {
  const choice = read('../src/components/OSUpdatesChoice.tsx');
  assert.match(choice, /originPhrases\(config\.covers/);
  assert.match(choice, /<details[^>]*data-os-origins-raw/);
  assert.doesNotMatch(choice, /\(config\.covers \?\? \[\]\)\.join\(/);
  const card = read('../src/components/SecurityUpdatesCard.tsx');
  assert.match(card, /life\.updates\.auto\.onPlain/);
  assert.match(card, /life\.updates\.reboot\.waiting/);
  for (const lang of ['en', 'tr']) {
    const words = read(`../src/i18n/screens/lifecycle/${lang}.ts`);
    for (const key of ['security', 'release', 'updates', 'backports', 'proposed', 'esm', 'vendor', 'other', 'distro', 'details']) {
      assert.match(words, new RegExp(`'life\\.origin\\.${key}'`), `${lang} life.origin.${key}`);
    }
  }
});

test('every SSH card word exists in Turkish too, and the card asks for every reason it can receive', async () => {
  const tr = read('../src/i18n/sshPassword/tr.ts');
  for (const key of Object.keys(enSSHPassword)) assert.ok(tr.includes(`'${key}'`), `tr ${key}`);
  const card = read('../src/components/SSHPasswordLoginCard.tsx');
  for (const reason of ['include_order', 'match_block', 'no_key_account', 'pubkey_off', 'drop_in_edited', 'drop_in_dir_missing', 'config_unreadable']) {
    assert.ok(enSSHPassword[`sshpw.blocked.${reason}`], reason);
    assert.match(card, new RegExp(`'${reason}'`));
  }
  for (const reason of ['validate_failed', 'not_effective', 'reload_failed', 'confirm_failed', 'write_failed', 'remove_failed', 'changed', 'config_invalid']) {
    assert.ok(enSSHPassword[`sshpw.reason.${reason}`], reason);
  }
  // The confirmation tells the owner to keep the session open and test a key first.
  assert.match(enSSHPassword['sshpw.confirmMessage'], /Keep the SSH session/);
  assert.match(enSSHPassword['sshpw.confirmMessage'], /NEW terminal/);
  // No browser dialogs; the product's own confirmation and progress.
  assert.doesNotMatch(card, /window\.confirm|\balert\(/);
  assert.match(card, /confirmAction\(/);
  assert.match(card, /sendWithSteps\(/);
});

test('the SSH card is its own chunk on the Security page', () => {
  const page = read('../src/components/SecurityPage.tsx');
  assert.match(page, /lazy\(\(\) => import\('\.\/SSHPasswordLoginCard'\)/);
  assert.doesNotMatch(page, /^import .*SSHPasswordLoginCard/m);
});

test('Problems now says an old reading as such', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.match(problems, /'stale'/);
  for (const lang of ['en', 'tr']) assert.match(read(`../src/i18n/screens/monitoring/${lang}.ts`), /'pnow\.reason\.stale'/);
});

test('the security audit shows the certificate names and the address check', () => {
  const audit = read('../src/components/SecurityAuditCard.tsx');
  assert.match(audit, /panel_tls_hostname_match/);
  assert.match(audit, /tls\.names/);
  for (const lang of ['en', 'tr']) {
    const words = read(`../src/i18n/screens/security/${lang}.ts`);
    for (const key of ['check.tlsHostname', 'code.tlsHostnameMatch', 'code.tlsHostnameMismatch', 'code.tlsHostnameAddress', 'tlsNames']) {
      assert.match(words, new RegExp(`'securityAudit\\.${key.replace('.', '\\.')}'`), `${lang} ${key}`);
    }
  }
});
