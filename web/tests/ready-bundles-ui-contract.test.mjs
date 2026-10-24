// Ready bundles (alpha.94, D-034) and the Components colour corrections that
// came with them; since alpha.95 (owner decision, 2026-10-17) every category
// shows the same bundles in the same order on every server, and only the state
// on each chip changes. The bundle words are checked against the Panel's own
// list of codes (bundleTextCodes in cmd/panel/bundles.go, which a Go test keeps
// complete), the decoder and the chips are run on payloads shaped like the
// owner's two servers, and the colour rules of DESIGN.md are pinned in the
// source.
// Hazır paketler: sözler Panel'in kod listesine karşı, çözücü ve çipler iki
// sunucu biçiminde, renk kuralları kaynakta denetlenir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en as enShellHalf } from '../src/i18n/en.ts';
import { enErrorsScreens as enErrorsPart } from '../src/i18n/screens/errors/en.ts';
// The coded refusals only screens show are the screen part "errors" since v0.1.0-alpha.98.
const en = { ...enShellHalf, ...enErrorsPart };
import { tr as trShellHalf } from '../src/i18n/tr.ts';
import { trErrorsScreens as trErrorsPart } from '../src/i18n/screens/errors/tr.ts';
const tr = { ...trShellHalf, ...trErrorsPart };
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';
import { enServerScreens } from './locale-catalogue.mjs';
import { trServerScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const transpile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const asModule = (code) => 'data:text/javascript;base64,' + Buffer.from(code).toString('base64');
// readyBundles names a mail bundle through the product's mail profile names.
const mailTextURL = asModule(transpile('../src/lib/mailProfileText.ts'));
const lib = await import(asModule(transpile('../src/lib/readyBundles.ts').replace(/from ['"]\.\/mailProfileText['"]/, `from '${mailTextURL}'`)));
const server = read('../../cmd/panel/bundles.go');
const strip = read('../src/components/ReadyBundleStrip.tsx');
const services = read('../src/components/ServiceList.tsx');
const ui = read('../src/components/ui.tsx');
const icons = read('../src/lib/componentIcons.tsx');
const design = read('../../DESIGN.md');
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

// bundleTextCodes in cmd/panel/bundles.go, resolved to their values.
const values = Object.fromEntries([...server.matchAll(/^\t(bundleText\w+)\s+bundleTextCode = "([a-z_]+)"$/gm)].map((m) => [m[1], m[2]]));
const listBody = /var bundleTextCodes = \[\]bundleTextCode\{([\s\S]*?)\n\}/.exec(server)?.[1] ?? '';
const codes = [...listBody.matchAll(/\b(bundleText\w+),/g)].map((m) => values[m[1]]);

test('every bundle the Panel can name has English and Turkish words, plain and with the same values', () => {
  assert.equal(codes.length, 6, `read ${codes.length} codes from the Panel`);
  assert.ok(codes.every(Boolean), JSON.stringify(codes));
  for (const code of codes) {
    for (const part of ['name', 'description', 'chip', 'about']) {
      const key = `services.bundles.${part}.${code}`;
      assert.ok(catalogues.en[key], `no English ${part} for ${code}`);
      assert.ok(catalogues.tr[key], `no Turkish ${part} for ${code}`);
      assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]), key);
      // Plain language: no internal names, no codes.
      for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale][key], /mail_profile|bundle\.|_|D-0/, `${locale} ${key}`);
    }
    // The "Add …" labels went with the "one more offer" rule (alpha.95).
    assert.equal(catalogues.en[`services.bundles.addon.${code}`], undefined, code);
  }
  // The owner's Turkish names.
  assert.equal(catalogues.tr['services.bundles.label'], 'Hazır paketler:');
  // The mail bundles carry the product's own mail profile names.
  for (const [code, key] of [['mail_core', 'coreMail'], ['mail_webmail', 'webmail'], ['mail_protected', 'protectedMail']]) {
    for (const locale of ['en', 'tr']) {
      assert.equal(catalogues[locale][`services.bundles.name.${code}`], catalogues[locale][`dashboard.audit.profile.${key}`], `${locale} ${code}`);
      assert.equal(catalogues[locale][`services.bundles.chip.${code}`], catalogues[locale][`dashboard.audit.profile.${key}`], `${locale} chip ${code}`);
    }
  }
  const t = (key, vars = {}) => Object.entries(vars).reduce((out, [k, v]) => out.split(`{${k}}`).join(String(v)), catalogues.tr[key] ?? key);
  assert.equal(lib.bundleName({ id: 'mail.webmail', name_code: 'mail_webmail', components: ['postfix'], action_route: 'mail_profile', profile_id: 'webmail' }, t, (id) => id), 'Webmail');
  assert.equal(lib.bundleName({ id: 'mail.core', name_code: 'mail_core', components: ['postfix'], action_route: 'mail_profile', profile_id: 'core-mail' }, t, (id) => id), 'Temel Posta');
  // alpha.101 (owner, 2026-10-22): a bundle is a service with its companion
  // tools; the database bundles say so in their names.
  const pgBundle = { id: 'db.postgresql', name_code: 'db_postgresql_phppgadmin', components: ['postgresql', 'nginx', 'php-fpm', 'phppgadmin'], action_route: 'components' };
  assert.equal(lib.bundleChipName(pgBundle, t, (id) => id), 'PostgreSQL + phpPgAdmin');
  assert.equal(catalogues.tr['services.bundles.name.db_postgresql_phppgadmin'], 'PostgreSQL + phpPgAdmin');
  assert.equal(catalogues.tr['services.bundles.name.db_mariadb_phpmyadmin'], 'MariaDB + phpMyAdmin');
  assert.equal(catalogues.en['services.bundles.chip.db_mariadb_phpmyadmin'], 'MariaDB + phpMyAdmin');
  assert.equal(catalogues.tr['services.bundles.name.web_nginx_php'], 'Nginx + PHP-FPM');
  // The retired one-component and add-on codes have no words: an older
  // Panel's answer is named by its components, never as a bundle with a tool.
  for (const code of ['db_postgresql', 'db_mariadb', 'db_postgresql_admin', 'db_mariadb_admin']) {
    for (const part of ['name', 'description', 'chip', 'about']) {
      for (const locale of ['en', 'tr']) assert.equal(catalogues[locale][`services.bundles.${part}.${code}`], undefined, `${locale} ${part} ${code}`);
    }
  }
  assert.equal(lib.bundleChipName({ id: 'db.postgresql', name_code: 'db_postgresql', components: ['postgresql'], action_route: 'components' }, t, (id) => id), 'postgresql');
  for (const [key, word] of [['repairEllipsis', 'Onar…'], ['continue', 'Devam et'], ['chip.install', 'Kur'], ['chip.add', 'Ekle'], ['chip.included', 'içinde']]) {
    assert.equal(catalogues.tr[`services.bundles.${key}`], word);
  }
  // Verify went with alpha.101: the page's Rescan reads the components again.
  for (const key of ['verify', 'verifyBusy', 'verifyScanning', 'installed.verifyHow']) {
    for (const locale of ['en', 'tr']) assert.equal(catalogues[locale][`services.bundles.${key}`], undefined, `${locale} ${key}`);
  }
  // Arch: phpPgAdmin is not offered, and the chip says so plainly.
  assert.equal(t('services.bundles.short.component_not_installable_named', { component: 'phpPgAdmin' }), 'phpPgAdmin bu sistemde kullanılamıyor');
  assert.equal(catalogues.en['services.bundles.short.component_not_installable_named'], '{component} is not available on this system');
  assert.equal(catalogues.en['services.bundles.chip.verifiedLabel'], '{name}, installed and verified');
  // Natural word order (alpha.95 review).
  assert.equal(catalogues.en['services.bundles.step'], '{n}/{total} · installing {component}');
  assert.equal(catalogues.tr['services.bundles.chip.progress'], '{n}/{total} bileşen kurulu');
  // An installed bundle is described, not told to install (alpha.95 review).
  assert.equal(catalogues.en['services.bundles.about.db_postgresql_phppgadmin'], 'PostgreSQL for your sites’ databases, with phpPgAdmin to manage them in a browser.');
  for (const code of codes) for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale][`services.bundles.about.${code}`], /^(Installs|Adds)\b|kurar\.$|ekler\.$/, `${locale} ${code}`);
  assert.equal(catalogues.tr['services.stopped'], 'Durdu');
  // Every services.bundles / dashboard.bundles key is in both languages.
  for (const prefix of ['services.bundles.', 'dashboard.bundles.', 'firewall.compact.']) {
    const enKeys = Object.keys(catalogues.en).filter((key) => key.startsWith(prefix)).sort();
    const trKeys = Object.keys(catalogues.tr).filter((key) => key.startsWith(prefix)).sort();
    assert.deepEqual(trKeys, enKeys, prefix);
    for (const key of enKeys) assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]), key);
  }
  // The permanent hint sentence under the strip is gone.
  assert.equal(catalogues.en['services.bundles.hint'], undefined);
});

// --- The decoder ------------------------------------------------------------------
const ids = new Set(['postfix', 'dovecot', 'rspamd', 'nginx', 'php-fpm', 'roundcube', 'mariadb', 'phpmyadmin', 'postgresql', 'phppgadmin']);
// The name codes the Panel sends (bundleTextCodes); the database bundles carry
// their tool in theirs since alpha.101.
const nameCodes = { 'db.postgresql': 'db_postgresql_phppgadmin', 'db.mariadb': 'db_mariadb_phpmyadmin' };
const make = (id, category, components, extra = {}) => ({
  id, category, name_code: nameCodes[id] ?? id.replace(/\./g, '_'), components, status: 'available', action: 'install', action_route: 'components', addons: [], missing_components: components, ...extra,
});
const mail = (id, profile, components, extra = {}) => make(id, 'email', components, { action_route: 'mail_profile', profile_id: profile, ...extra });

test('the decoder takes only the contract and drops the list rather than offer what the Panel did not mean', () => {
  assert.deepEqual(lib.decodeReadyBundles(undefined, ids), [], 'an older Panel sends none');
  const good = [make('web.nginx_php', 'web', ['nginx', 'php-fpm'])];
  assert.equal(lib.decodeReadyBundles(good, ids).length, 1);
  for (const [why, broken] of [
    ['an action its state does not carry', [make('web.nginx_php', 'web', ['nginx'], { status: 'installed_unverified', action: 'install', missing_components: [] })]],
    ['a component the scan does not know', [make('web.nginx_php', 'web', ['nginx', 'caddy'])]],
    ['a missing component outside the bundle', [make('web.nginx_php', 'web', ['nginx'], { missing_components: ['mariadb'] })]],
    ['a mail bundle without its profile', [mail('mail.core', 'exchange', ['postfix'])]],
    ['a component bundle that repairs', [make('db.mariadb', 'database', ['mariadb'], { status: 'installed_verified', action: 'repair', missing_components: [] })]],
    ['a component bundle that is partial', [make('web.nginx_php', 'web', ['nginx', 'php-fpm'], { status: 'partial', action: 'continue', missing_components: ['php-fpm'] })]],
    ['a mail bundle that only verifies', [mail('mail.core', 'core-mail', ['postfix'], { status: 'installed_unverified', action: 'verify', missing_components: [] })]],
    ['an offer with nothing to install', [make('db.mariadb', 'database', ['mariadb'], { missing_components: [] })]],
    ['a duplicate', [...good, ...good]],
  ]) {
    assert.equal(lib.decodeReadyBundles(broken, ids), null, why);
  }
});

// alpha.101: an id the answer does not carry is ignored rather than a reason to
// drop the whole list, and an older Panel's answer (one-component database
// bundles, add-ons, Verify) is still read.
test('the decoder ignores ids it does not carry and still reads an older Panel', () => {
  const pg = make('db.postgresql', 'database', ['postgresql', 'nginx', 'php-fpm', 'phppgadmin'], { addons: ['db.postgresql.admin'] });
  const decoded = lib.decodeReadyBundles([pg], ids);
  assert.equal(decoded.length, 1);
  assert.deepEqual(decoded[0].addons, [], 'an add-on reference to an id the answer does not carry is dropped');
  const orphan = make('db.mariadb.admin', 'database', ['nginx', 'php-fpm', 'phpmyadmin'], { parent: 'db.mariadb' });
  assert.deepEqual(lib.decodeReadyBundles([pg, orphan], ids).map((bundle) => bundle.id), ['db.postgresql'], 'an add-on of a bundle that is not there is not drawn');
  // A bundle that offers that dropped add-on does not keep it in its add-ons.
  const offering = make('web.nginx_php', 'web', ['nginx', 'php-fpm'], { addons: ['db.mariadb.admin'] });
  const left = lib.decodeReadyBundles([pg, orphan, offering], ids);
  assert.deepEqual(left.map((bundle) => bundle.id), ['db.postgresql', 'web.nginx_php']);
  assert.deepEqual(left.find((bundle) => bundle.id === 'web.nginx_php').addons, [], 'a bundle kept an add-on that was itself left out');
  const older = [
    { ...make('db.mariadb', 'database', ['mariadb'], { addons: ['db.mariadb.admin'] }), name_code: 'db_mariadb', status: 'installed_unverified', action: 'verify', missing_components: [] },
    { ...make('db.mariadb.admin', 'database', ['nginx', 'php-fpm', 'phpmyadmin'], { parent: 'db.mariadb' }), name_code: 'db_mariadb_admin' },
  ];
  const read = lib.decodeReadyBundles(older, ids);
  assert.deepEqual(read.map((bundle) => [bundle.id, bundle.action]), [['db.mariadb', 'verify'], ['db.mariadb.admin', 'install']]);
});

// --- The owner's two servers (screens of 2026-10-17) ---------------------------------
// Boston: Nginx, Node, PowerDNS, PostgreSQL, Fail2ban, certbot, nftables and
// cron installed; no mail, no MariaDB, no PHP-FPM.
// Frankfurt: every bundle installed and verified (Webmail and Spam-Protected
// Mail, both databases with their web tools, Nginx + PHP-FPM).
const verified = (bundle, action = bundle.action_route === 'mail_profile' ? 'repair' : 'none') => ({ ...bundle, status: 'installed_verified', action, missing_components: [] });
// The registry of alpha.101 (cmd/panel/bundles.go): a database bundle is the
// database with its browser tool and the web server and PHP-FPM the tool runs on.
const registry = [
  mail('mail.core', 'core-mail', ['postfix', 'dovecot'], { addons: ['mail.protected'] }),
  mail('mail.webmail', 'webmail', ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], { addons: ['mail.protected'] }),
  mail('mail.protected', 'protected-mail', ['postfix', 'dovecot', 'rspamd']),
  make('db.postgresql', 'database', ['postgresql', 'nginx', 'php-fpm', 'phppgadmin']),
  make('db.mariadb', 'database', ['mariadb', 'nginx', 'php-fpm', 'phpmyadmin']),
  make('web.nginx_php', 'web', ['nginx', 'php-fpm']),
];
// Boston as the Panel derives it (TestBundlesOnABostonShapedServer):
// PostgreSQL and Nginx are installed, so PostgreSQL + phpPgAdmin is offered
// and installs only PHP-FPM and phpPgAdmin.
const boston = registry.map((bundle) => {
  switch (bundle.id) {
  case 'db.postgresql': return { ...bundle, missing_components: ['php-fpm', 'phppgadmin'] };
  case 'db.mariadb': return { ...bundle, missing_components: ['mariadb', 'php-fpm', 'phpmyadmin'] };
  case 'web.nginx_php': return { ...bundle, missing_components: ['php-fpm'] };
  default: return bundle;
  }
});
const frankfurt = registry.map((bundle) => verified(bundle));
const t = (key, vars = {}) => Object.entries(vars).reduce((out, [k, v]) => out.split(`{${k}}`).join(String(v)), catalogues.en[key] ?? key);
const chipsOf = (all, category) => lib.bundleStripModel(all, category).chips.map((chip) => [chip.bundle.id, chip.state]);

test('every category shows the same bundles in the same order on both servers; only the states differ', () => {
  for (const all of [boston, frankfurt]) assert.equal(lib.decodeReadyBundles(all, ids).length, 6);
  for (const category of ['email', 'database', 'web']) {
    const order = registry.filter((bundle) => bundle.category === category).map((bundle) => bundle.id);
    assert.deepEqual(chipsOf(boston, category).map(([id]) => id), order, `Boston ${category}`);
    assert.deepEqual(chipsOf(frankfurt, category).map(([id]) => id), order, `Frankfurt ${category}`);
  }
  assert.deepEqual(chipsOf(boston, 'database'), [
    ['db.postgresql', 'available'], ['db.mariadb', 'available'],
  ], 'PostgreSQL alone is not the bundle: PostgreSQL + phpPgAdmin is offered on Boston');
  assert.deepEqual(chipsOf(frankfurt, 'database'), [['db.postgresql', 'verified'], ['db.mariadb', 'verified']]);
  assert.deepEqual(chipsOf(boston, 'email'), [['mail.core', 'available'], ['mail.webmail', 'available'], ['mail.protected', 'available']]);
  assert.deepEqual(chipsOf(frankfurt, 'email'), [['mail.core', 'included'], ['mail.webmail', 'verified'], ['mail.protected', 'verified']], 'Core Mail is included, not hidden');
  const core = lib.bundleStripModel(frankfurt, 'email').chips[0];
  assert.equal(core.includedIn.id, 'mail.webmail');
  assert.equal(lib.bundleStripModel(boston, 'dns').empty, true);
});

// Arch (pacman): phpPgAdmin has no package there, so the Panel blocks the
// whole bundle (TestBundlesOnAnArchShapedServer) and never offers a subset.
test('a bundle with a part this system does not offer is blocked whole, and its chip names that part', () => {
  const arch = registry.map((bundle) => (bundle.id === 'db.postgresql'
    ? { ...bundle, status: 'blocked', action: 'none', blocked_code: 'component_not_installable', blocked_vars: { component: 'phpPgAdmin', kind: 'package', platform: 'Arch Linux (pacman)' } }
    : bundle));
  assert.equal(lib.decodeReadyBundles(arch, ids).length, 6);
  assert.deepEqual(chipsOf(arch, 'database'), [['db.postgresql', 'blocked'], ['db.mariadb', 'available']]);
  assert.equal(t('services.bundles.short.component_not_installable_named', { component: 'phpPgAdmin' }), 'phpPgAdmin is not available on this system');
  assert.equal(lib.bundlesSummary(arch).installable, 5, 'a blocked bundle is not counted as installable');
  // The strip's short reason, as the chip draws it.
  assert.match(strip, /if \(bundle\.blocked_code === 'component_requires' && vars\.required\) return t\('services\.bundles\.short\.component_requires', \{ required: requirementText\(vars\.required, t\) \}\);/);
  assert.match(strip, /\? t\('services\.bundles\.short\.component_not_installable_named', \{ component: vars\.component \}\)/);
  // A blocked chip installs nothing; it is a quiet button whose press shows
  // the full reason under the chips, so keyboard and touch reach it, not only
  // a tooltip (alpha.95 review).
  const blockedCase = strip.slice(strip.indexOf("case 'blocked':"), strip.indexOf('default:', strip.indexOf("case 'blocked':")));
  assert.match(blockedCase, /className=\{`\$\{quietChip\} \$\{chipFocus\}/);
  assert.match(blockedCase, /aria-expanded=\{reasonShown === bundle\.id\}/);
  assert.match(blockedCase, /aria-controls=\{`bundle-reason-/);
  assert.match(blockedCase, /onClick=\{\(\) => setReasonShown\(reasonShown === bundle\.id \? null : bundle\.id\)\}/);
  assert.doesNotMatch(blockedCase, /onMail|onComponents|act\(/);
  assert.match(strip, /data-bundle-blocked-reason=\{shown\.bundle\.id\}/);
  // An add-on (an older Panel's) says Add, a top-level bundle Install.
  assert.match(strip, /t\(bundle\.parent !== undefined \? 'services\.bundles\.chip\.add' : 'services\.bundles\.chip\.install'\)/);
});

test('the Dashboard counts what the chips show: every registry entry, blocked not installable', () => {
  // Boston: nothing installed and verified (PostgreSQL alone is not its
  // bundle), six can be installed; Frankfurt: all six installed and verified.
  for (const [all, verifiedCount, installable] of [[boston, 0, 6], [frankfurt, 6, 0]]) {
    const summary = lib.bundlesSummary(all);
    const chips = ['email', 'database', 'web'].flatMap((category) => lib.bundleStripModel(all, category).chips);
    assert.equal(summary.installable, chips.filter((chip) => chip.state === 'available' || chip.state === 'partial').length);
    assert.equal(summary.verified, chips.filter((chip) => chip.state === 'verified' || chip.state === 'included').length);
    assert.equal(summary.installable, installable);
    assert.equal(summary.verified, verifiedCount);
    assert.equal(summary.known, true);
  }
  assert.equal(t('dashboard.bundles.summary', { verified: 6, installable: 0 }), '6 installed and verified · 0 can be installed');
  // Boston's card line, from the same count.
  assert.equal(t('dashboard.bundles.summaryInstallable', { installable: lib.bundlesSummary(boston).installable }), 'None installed yet · 6 can be installed');
});

test('a mail bundle partial only through parts an installed bundle shares is offered as Install; one attempted keeps Continue', () => {
  const beside = registry.map((bundle) => {
    if (bundle.id === 'mail.webmail') return verified(bundle);
    if (bundle.id === 'mail.protected') return { ...bundle, status: 'partial', action: 'continue', missing_components: ['rspamd'] };
    return bundle;
  });
  assert.equal(lib.bundleChipState(beside.find((bundle) => bundle.id === 'mail.protected'), beside), 'available');
  const attempted = beside.map((bundle) => (bundle.id === 'mail.protected' ? { ...bundle, latest_attempt_status: 'failed' } : bundle));
  assert.equal(lib.bundleChipState(attempted.find((bundle) => bundle.id === 'mail.protected'), attempted), 'partial');
  assert.deepEqual(lib.bundleStripModel(attempted, 'email').noted.map((bundle) => bundle.id), ['mail.protected']);
});

test('a host-wide refusal is said once for the category, and a stale scan claims nothing', () => {
  const dns = boston.map((bundle) => (bundle.category === 'email' ? { ...bundle, status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' } : bundle));
  const email = lib.bundleStripModel(dns, 'email');
  assert.equal(email.hostBlocked, 'dns_identity_not_ready');
  assert.deepEqual(email.chips.map((chip) => chip.state), ['blocked', 'blocked', 'blocked']);
  const unknown = boston.map((bundle) => ({ ...bundle, status: 'unknown', action: 'none', blocked_code: 'state_unverified' }));
  const model = lib.bundleStripModel(unknown, 'database');
  assert.equal(model.unknown, true);
  assert.deepEqual(model.chips, [], 'no chip may act on a stale state');
  assert.equal(lib.bundlesSummary(unknown).known, false);
  // One stale bundle is enough: the whole strip says one line.
  const oneStale = boston.map((bundle) => (bundle.id === 'db.mariadb' ? { ...bundle, status: 'unknown', action: 'none', blocked_code: 'state_unverified', missing_components: ['mariadb'] } : bundle));
  assert.equal(lib.bundleStripModel(oneStale, 'database').unknown, true);
  assert.match(strip, /data-bundle-unknown=''>\{t\('services\.bundles\.unknown'\)\}/);
});

test('an installed mail bundle stays visible beside a host-wide refusal', () => {
  // As the Panel derives it: Core Mail's parts are Webmail's, so it is
  // installed and verified beside the refusal too.
  const dns = boston.map((bundle) => {
    if (bundle.id === 'mail.webmail' || bundle.id === 'mail.core') return { ...bundle, status: 'installed_verified', action: 'none', missing_components: [] };
    if (bundle.category === 'email') return { ...bundle, status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' };
    return bundle;
  });
  assert.equal(lib.decodeReadyBundles(dns, ids).length, 6);
  const email = lib.bundleStripModel(dns, 'email');
  assert.equal(email.hostBlocked, 'dns_identity_not_ready');
  assert.deepEqual(email.chips.map((chip) => [chip.bundle.id, chip.state]), [['mail.core', 'included'], ['mail.webmail', 'verified'], ['mail.protected', 'blocked']]);
});

test('"included" never hides a bundle\'s own failed or running attempt, and the Dashboard does not count it verified', () => {
  for (const attempt of ['failed', 'in_progress']) {
    const own = frankfurt.map((bundle) => (bundle.id === 'mail.core' ? { ...bundle, latest_attempt_status: attempt } : bundle));
    const chip = lib.bundleStripModel(own, 'email').chips.find((item) => item.bundle.id === 'mail.core');
    assert.equal(chip.state, 'unverified', attempt);
    assert.equal(chip.includedIn, undefined);
    assert.deepEqual(lib.bundleStripModel(own, 'email').noted.map((bundle) => bundle.id), ['mail.core'], attempt);
    const summary = lib.bundlesSummary(own);
    assert.equal(summary.verified, 5, attempt);
    assert.deepEqual(summary.unverified.map((bundle) => bundle.id), ['mail.core'], attempt);
  }
  // A contained bundle that is not itself installed and verified is not "included".
  const notOwn = frankfurt.map((bundle) => (bundle.id === 'mail.core' ? { ...bundle, status: 'installed_unverified', action: 'repair' } : bundle));
  assert.equal(lib.bundleChipState(notOwn.find((bundle) => bundle.id === 'mail.core'), notOwn), 'unverified');
});

test('the "some parts are already present" note is never said under the chips; warnings only on unverified or partial chips', () => {
  const present = boston.map((bundle) => (bundle.id === 'mail.webmail' ? { ...bundle, warning_code: 'partly_present' } : bundle));
  assert.deepEqual(lib.bundleStripModel(present, 'email').noted, []);
  const partial = registry.map((bundle) => (bundle.id === 'mail.protected'
    ? { ...bundle, status: 'partial', action: 'continue', missing_components: ['rspamd'], warning_code: 'partly_present' }
    : bundle));
  assert.equal(lib.bundleChipState(partial.find((bundle) => bundle.id === 'mail.protected'), partial), 'partial');
  assert.deepEqual(lib.bundleStripModel(partial, 'email').noted, [], 'partly_present is not a problem line');
  const unverifiedWarn = frankfurt.map((bundle) => (bundle.id === 'mail.webmail' ? { ...bundle, status: 'installed_unverified', warning_code: 'roundcube_not_ready' } : bundle));
  assert.deepEqual(lib.bundleStripModel(unverifiedWarn, 'email').noted.map((bundle) => bundle.id), ['mail.webmail']);
  const verifiedWarn = frankfurt.map((bundle) => (bundle.id === 'mail.webmail' ? { ...bundle, warning_code: 'repair_to_verify' } : bundle));
  assert.deepEqual(lib.bundleStripModel(verifiedWarn, 'email').noted, [], 'a verified chip carries no warning line');
});

test('a failed or running mail attempt is said again under the chips (D-024)', () => {
  const failed = boston.map((bundle) => (bundle.id === 'mail.webmail' ? { ...bundle, latest_attempt_status: 'failed' } : bundle));
  const model = lib.bundleStripModel(failed, 'email');
  assert.deepEqual(model.noted.map((bundle) => bundle.id), ['mail.webmail']);
  assert.match(strip, /if \(bundle\.latest_attempt_status === 'in_progress'\) return t\('services\.bundles\.attemptRunning'\);/);
  assert.match(strip, /if \(bundle\.latest_attempt_status === 'failed'\) return \(profile \? mailProfileAttemptError\(profile, t\) : ''\) \|\| t\('services\.bundles\.lastAttemptFailed'\);/);
  assert.match(strip, /if \(bundle\.warning_code\) return mailProfileWarning\(/);
});

// --- Shape, colour and the one primary ------------------------------------------------
test('the strip is chips only: no filled button, no Repair link on the strip, no hint, amber only for a webmail runtime not ready', () => {
  const stripOnly = strip.slice(strip.indexOf('export function ReadyBundleStrip'), strip.indexOf('export function BundleInstalledDialog'));
  assert.doesNotMatch(stripOnly, /variant='primary'|bg-primary text-primary-fg/);
  assert.equal((stripOnly.match(/text-warning/g) ?? []).length, 1);
  assert.match(stripOnly, /bundle\.warning_code === 'roundcube_not_ready'/);
  assert.doesNotMatch(stripOnly, /warning-mark|bg-warning/);
  // Repair and Verify live in the installed dialogue, not on the strip.
  assert.doesNotMatch(stripOnly, /services\.bundles\.repair|<Button/);
  assert.doesNotMatch(strip, /services\.bundles\.hint/);
  // Installed chips open the dialogue; a verified chip has an accessible name in words.
  assert.match(stripOnly, /onClick=\{\(\) => setDetails\(bundle\.id\)\}/);
  assert.match(stripOnly, /t\('services\.bundles\.chip\.verifiedLabel', \{ name: name\(bundle\) \}\)/);
  // The wait moves (D-033), and says "installing" only once the server accepted.
  assert.match(stripOnly, /update-step-indeterminate/);
  assert.match(stripOnly, /role='status' aria-live='polite'/);
  assert.match(stripOnly, /run\.phase === 'running'\s*\n\s*\? t\('services\.bundles\.step'/);
  // The installed dialogue describes the bundle; it never says "Installs …".
  const installedDialog = strip.slice(strip.indexOf('export function BundleInstalledDialog'), strip.indexOf('export function BundleInstallDialog'));
  assert.match(installedDialog, /description=\{bundleAbout\(bundle, t\)/);
  assert.doesNotMatch(installedDialog, /bundleDescription/);
  assert.match(stripOnly, /: t\('services\.bundles\.checkingBefore', \{ component \}\)/);
  // The one-line notices replaced the mail boxes.
  assert.doesNotMatch(services, /data-mail-dns-secondary=''[^>]*rounded-xl border border-warning/);
  // Chips stay left after the label and wrap: nothing pushes them right.
  assert.doesNotMatch(stripOnly, /ml-auto/);
});

// alpha.101 (owner's screens on Frankfurt): each part says its row's state and
// version; Verify and its sentence are gone (the page's Rescan reads again);
// a mail bundle keeps Repair, which does real work.
test('the installed dialogue says each part\'s state and version, offers Repair for a mail bundle and no Verify', () => {
  const dialog = strip.slice(strip.indexOf('export function BundleInstalledDialog'), strip.indexOf('export function BundleInstallDialog'));
  assert.match(dialog, /\{mail && !includedIn && \(/);
  assert.doesNotMatch(dialog, /services\.bundles\.verify|verifyHeld|onVerify|ScanSearch/);
  assert.doesNotMatch(dialog, /services\.bundles\.dialog\.alreadyInstalled/);
  assert.match(dialog, /const word = componentStateWord\(service\);/);
  assert.match(dialog, /const version = componentVersionText\(service\);/);
  assert.match(dialog, /t\('services\.bundles\.installed\.unverifiedHow', \{ rescan: t\('services\.rescan'\) \}\)/);
  assert.match(dialog, /data-bundle-part-state=\{part\.word\}/);
  assert.doesNotMatch(strip, /verifyHeld|onVerify/);
  assert.doesNotMatch(services, /bundleVerifyHeld|verifyHeld=|onVerify=/);
});

test('the gate between bundle steps reads the unverified flag through a ref set when it changes', () => {
  assert.match(services, /const stateUnverifiedRef = useRef\(verification\.unverified\);/);
  assert.match(services, /const markStateUnverified = \(cause: UnverifiedCause = unverifiedByAction\) => \{\s*\n\s*stateUnverifiedRef\.current = true;/);
  assert.match(services, /stateUnverifiedRef\.current = false;\s*\n\s*persistServiceVerificationState\(next\);/);
  const gate = services.slice(services.indexOf('const bundleGate = async'), services.indexOf('const installedNow ='));
  assert.equal((gate.match(/stateUnverifiedRef\.current/g) ?? []).length, 2, 'read before and after the readiness read');
  assert.doesNotMatch(gate, /if \(stateUnverified\)/);
});

test('a status dot has four tones, and the Components rows use them', () => {
  assert.match(ui, /export type StatusDotTone = 'success' \| 'danger' \| 'idle' \| 'unknown';/);
  assert.match(ui, /idle: 'border border-fg-subtle bg-transparent'/);
  assert.match(ui, /danger: 'bg-danger'/);
  // alpha.104: a PHP tool whose PHP is missing is not ready, in the danger tone.
  assert.match(services, /<StatusDot tone=\{notChecked\(s\) \? 'unknown' : !s\.is_installed \? 'idle' : componentStateWord\(s\) === 'notReady' \? 'danger' : \(running \|\| s\.kind === 'tool' \|\| s\.status === 'installed'\) \? 'success' : 'danger'\} \/>/);
  assert.match(services, /<StatusDot tone=\{instRunning \? 'success' : 'danger'\} \/>/);
  assert.match(design, /StatusDot/);
});

test('row actions share one outlined shape, neutral at rest; Stop is a stop sign, not a square', () => {
  const action = services.slice(services.indexOf('function ActionIcon('), services.indexOf('\n}\n', services.indexOf('function ActionIcon(')));
  assert.match(action, /text-fg-muted/);
  assert.match(action, /rounded-lg border border-border-strong bg-surface p-1\.5/);
  for (const line of action.split('\n').filter((entry) => /^\s+(success|danger|warning): '/.test(entry))) {
    for (const cls of line.split("'")[1].split(/\s+/)) {
      assert.match(cls, /^(hover|focus-visible):/, `"${cls}" colours an icon at rest`);
    }
  }
  // On a phone the name is written and the target is 44 px.
  assert.match(action, /max-sm:min-h-11/);
  assert.match(action, /<span className="sm:sr-only">\{title\}<\/span>/);
  // Stop: a stop sign in a circle everywhere; no empty square that reads as a checkbox.
  assert.doesNotMatch(services, /\bSquare\b/);
  assert.equal((services.match(/<StopCircle className="h-4 w-4" aria-hidden="true" \/>/g) ?? []).length, 2);
  // Delete is the same ActionIcon as Restart and Stop.
  assert.match(services, /<ActionIcon\s*\n\s*title=\{t\('services\.uninstall'\)\}\s*\n\s*onClick=\{\(\) => openRemovalFlow\(s\)\}\s*\n\s*tone="danger"\s*\n\s*>\s*\n\s*<Trash2 className="h-4 w-4" aria-hidden="true" \/>/);
  assert.match(services, /variant=\{actionName === 'stop' \? 'danger' : 'primary'\}/);
});

test('icons come from one map and the emoji is drawn nowhere', () => {
  assert.match(icons, /export function componentIcon\(id: string, category: string\): LucideIcon/);
  assert.match(icons, /componentIconMap\[id\] \?\? categoryIcons\[category\] \?\? Boxes/);
  assert.doesNotMatch(services, /\{s\.icon\}|\{service\.icon\}/);
  assert.match(services, /icon=\{componentIcon\(service\.id, service\.category\)\}/);
  assert.match(services, /<RowIcon id=\{s\.id\} category=\{s\.category\}/);
});

// alpha.97 (D-036): the desktop version column returns only when the package
// database gave at least one installed component a version; otherwise the
// rows keep alpha.95's three columns and the version under the description.
test('rows align in three columns, or four when a package version is known; never a column of dashes', () => {
  assert.match(services, /const versionColumn = services\.some\(\(s\) => packageVersionFacts\(s\) !== null\);/);
  assert.match(services, /versionColumn \? 'md:grid-cols-\[minmax\(0,1fr\)_7\.5rem_8rem_17rem\]' : 'md:grid-cols-\[minmax\(0,1fr\)_8rem_17rem\]'/);
  assert.doesNotMatch(services, /hidden w-28 shrink-0 sm:block/, 'the desktop version column of dashes is gone');
  assert.doesNotMatch(services, /\?\? '—'/, 'an unknown version is never a dash');
  assert.match(services, /<RowVersion service=\{s\} className=\{versionColumn \? 'md:hidden' : ''\} \/>/);
  assert.match(services, /\{versionColumn && \(\s*\n\s*<div className="hidden min-w-0 md:block" data-version-column="">/);
  assert.match(services, /const rowButtonClass = '[^']*border border-border-strong bg-surface[^']*max-sm:min-h-11[^']*'/);
  assert.doesNotMatch(services, /rounded-lg bg-primary px-3 py-1\.5 text-xs font-semibold text-primary-fg/);
  // A requirement badge never breaks a name such as PHP-FPM at its hyphen.
  assert.match(services, /<span><KeepWords text=\{t\('services\.requiresLabel', \{ names: reqNames\(s\.requires_missing\) \}\)\} \/><\/span>/);
});

test('a healthy firewall is one calm line; the box stays for every other state', () => {
  // The firewall box has its own module since v0.1.0-alpha.98 (D-037).
  const firewallPanel = read('../src/components/FirewallPanel.tsx');
  const bar = firewallPanel.slice(firewallPanel.indexOf('function FirewallBar('), firewallPanel.indexOf('function FirewallActionConfirmationDialog('));
  assert.ok(bar.length > 0);
  assert.match(services, /import \{ FirewallBar \} from '\.\/FirewallPanel';/);
  assert.match(bar, /const healthy = st\.enabled\s*\n\s*&& \(st\.persistence_state === undefined \|\| st\.persistence_state === '' \|\| st\.persistence_state === 'ready'\)\s*\n\s*&& !sshReason;/);
  assert.match(bar, /if \(healthy && place === 'components'\) \{/);
  assert.match(bar, /data-firewall-compact=""/);
  assert.match(bar, /t\('firewall\.compact\.settings'\)/);
  // Its settings are on Server → Security since v0.1.0-alpha.98 (D-037): the
  // calm line's link goes there and the box no longer opens on Components.
  assert.match(bar, /to="\/security#firewall"/);
  assert.doesNotMatch(bar, /setExpanded|firewall\.compact\.less/);
  // The box keeps "off" in the attention colour and is no longer green when on.
  assert.doesNotMatch(bar, /border-success\/30 bg-success\/5/);
});
