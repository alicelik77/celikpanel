// Ready bundles (alpha.94, D-034) and the Components colour corrections that
// came with them. The bundle words are checked against the Panel's own list of
// codes (bundleTextCodes in cmd/panel/bundles.go, which a Go test keeps
// complete), the decoder and the strip's choices are run on the owner's two
// servers, and the colour rules of DESIGN.md are pinned in the source.
// Hazır paketler ve onlarla gelen renk düzeltmeleri: sözler Panel'in kod
// listesine karşı, çözücü ve şerit kararları iki sunucu üzerinde, renk
// kuralları kaynakta denetlenir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

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
  assert.equal(codes.length, 8, `read ${codes.length} codes from the Panel`);
  assert.ok(codes.every(Boolean), JSON.stringify(codes));
  for (const code of codes) {
    for (const part of ['name', 'description', 'addon']) {
      const key = `services.bundles.${part}.${code}`;
      assert.ok(catalogues.en[key], `no English ${part} for ${code}`);
      assert.ok(catalogues.tr[key], `no Turkish ${part} for ${code}`);
      assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]), key);
      // Plain language: no internal names, no codes.
      for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale][key], /mail_profile|bundle\.|_|D-0/, `${locale} ${key}`);
    }
  }
  // The owner's Turkish names.
  assert.equal(catalogues.tr['services.bundles.label'], 'Hazır paketler:');
  // The mail bundles carry the product's own mail profile names.
  for (const [code, key] of [['mail_core', 'coreMail'], ['mail_webmail', 'webmail'], ['mail_protected', 'protectedMail']]) {
    for (const locale of ['en', 'tr']) {
      assert.equal(catalogues[locale][`services.bundles.name.${code}`], catalogues[locale][`dashboard.audit.profile.${key}`], `${locale} ${code}`);
    }
  }
  const t = (key, vars = {}) => Object.entries(vars).reduce((out, [k, v]) => out.split(`{${k}}`).join(String(v)), catalogues.tr[key] ?? key);
  assert.equal(lib.bundleName({ id: 'mail.webmail', name_code: 'mail_webmail', components: ['postfix'], action_route: 'mail_profile', profile_id: 'webmail' }, t, (id) => id), 'Webmail');
  assert.equal(lib.bundleName({ id: 'mail.core', name_code: 'mail_core', components: ['postfix'], action_route: 'mail_profile', profile_id: 'core-mail' }, t, (id) => id), 'Temel Posta');
  assert.equal(catalogues.tr['services.bundles.unverified'], '{name}: kurulu, doğrulanmadı');
  assert.equal(catalogues.tr['services.bundles.name.db_postgresql'], 'PostgreSQL paketi');
  assert.equal(catalogues.tr['services.bundles.name.db_mariadb'], 'MariaDB paketi');
  assert.equal(catalogues.tr['services.bundles.name.web_nginx_php'], 'Nginx + PHP-FPM');
  assert.equal(catalogues.tr['services.bundles.addon.mail_protected'], 'Spam koruması ekle');
  assert.equal(catalogues.tr['services.bundles.addon.db_mariadb_admin'], 'phpMyAdmin ekle');
  assert.equal(catalogues.tr['services.bundles.addon.db_postgresql_admin'], 'phpPgAdmin ekle');
  for (const [key, word] of [['verify', 'Doğrula'], ['repair', 'Onar'], ['continue', 'Devam et'], ['installEllipsis', 'Kur…']]) {
    assert.equal(catalogues.tr[`services.bundles.${key}`], word);
  }
  assert.equal(catalogues.tr['services.stopped'], 'Durdu');
  // Every services.bundles / dashboard.bundles key is in both languages.
  for (const prefix of ['services.bundles.', 'dashboard.bundles.', 'firewall.compact.']) {
    const enKeys = Object.keys(catalogues.en).filter((key) => key.startsWith(prefix)).sort();
    const trKeys = Object.keys(catalogues.tr).filter((key) => key.startsWith(prefix)).sort();
    assert.deepEqual(trKeys, enKeys, prefix);
    for (const key of enKeys) assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]), key);
  }
});

// --- The decoder ------------------------------------------------------------------
const ids = new Set(['postfix', 'dovecot', 'rspamd', 'nginx', 'php-fpm', 'roundcube', 'mariadb', 'phpmyadmin', 'postgresql', 'phppgadmin']);
const make = (id, category, components, extra = {}) => ({
  id, category, name_code: id.replace(/\./g, '_'), components, status: 'available', action: 'install', action_route: 'components', addons: [], missing_components: components, ...extra,
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
    ['an add-on of an unknown bundle', [make('db.mariadb', 'database', ['mariadb'], { addons: ['db.mariadb.admin'] })]],
    ['an offer with nothing to install', [make('db.mariadb', 'database', ['mariadb'], { missing_components: [] })]],
    ['a duplicate', [...good, ...good]],
  ]) {
    assert.equal(lib.decodeReadyBundles(broken, ids), null, why);
  }
});

// --- What a strip shows, on the owner's two servers ---------------------------------
const boston = [
  mail('mail.core', 'core-mail', ['postfix', 'dovecot'], { addons: ['mail.protected'] }),
  mail('mail.webmail', 'webmail', ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], { addons: ['mail.protected'] }),
  mail('mail.protected', 'protected-mail', ['postfix', 'dovecot', 'rspamd']),
  make('db.postgresql', 'database', ['postgresql'], { addons: ['db.postgresql.admin'] }),
  make('db.mariadb', 'database', ['mariadb'], { addons: ['db.mariadb.admin'] }),
  make('db.postgresql.admin', 'database', ['nginx', 'php-fpm', 'phppgadmin'], { parent: 'db.postgresql', status: 'blocked', action: 'none', blocked_code: 'component_requires' }),
  make('db.mariadb.admin', 'database', ['nginx', 'php-fpm', 'phpmyadmin'], { parent: 'db.mariadb', status: 'blocked', action: 'none', blocked_code: 'component_requires' }),
  make('web.nginx_php', 'web', ['nginx', 'php-fpm']),
];
const installed = (bundle, status = 'installed_verified', action = bundle.action_route === 'mail_profile' ? 'repair' : 'none') => ({ ...bundle, status, action, missing_components: [] });
const frankfurt = boston.map((bundle) => {
  switch (bundle.id) {
  case 'mail.core': return installed(bundle);
  case 'mail.webmail': return installed(bundle);
  case 'mail.protected': return { ...bundle, status: 'partial', action: 'continue', missing_components: ['rspamd'] };
  case 'db.mariadb': return installed(bundle);
  case 'db.mariadb.admin': return { ...bundle, status: 'available', action: 'install', missing_components: ['phpmyadmin'], blocked_code: undefined };
  case 'web.nginx_php': return installed(bundle);
  default: return bundle;
  }
});

test('Boston, nothing installed: every top-level bundle is offered; an add-on waits for its parent', () => {
  assert.deepEqual(lib.decodeReadyBundles(boston, ids).length, 8);
  const email = lib.bundleStripModel(boston, 'email');
  assert.deepEqual(email.offers.map((offer) => [offer.bundle.id, offer.add]), [['mail.core', false], ['mail.webmail', false], ['mail.protected', false]]);
  assert.deepEqual(email.installed, []);
  const database = lib.bundleStripModel(boston, 'database');
  assert.deepEqual(database.offers.map((offer) => offer.bundle.id), ['db.postgresql', 'db.mariadb']);
  assert.deepEqual(database.blocked, [], 'an add-on is not said before its parent is installed');
  assert.equal(lib.bundleStripModel(boston, 'dns').empty, true);
});

test('Frankfurt: webmail verified collapses to one line with spam protection as the one add-on; MariaDB offers phpMyAdmin', () => {
  const email = lib.bundleStripModel(frankfurt, 'email');
  assert.deepEqual(email.installed.map((bundle) => bundle.id), ['mail.webmail'], 'core mail is contained in webmail');
  assert.deepEqual(email.partial, [], 'shared parts are not a half-installed bundle');
  assert.deepEqual(email.offers.map((offer) => [offer.bundle.id, offer.add]), [['mail.protected', true]]);
  const database = lib.bundleStripModel(frankfurt, 'database');
  assert.deepEqual(database.installed.map((bundle) => bundle.id), ['db.mariadb']);
  assert.deepEqual(database.offers.map((offer) => [offer.bundle.id, offer.add]), [['db.mariadb.admin', true]], 'at most one more offer');
  const web = lib.bundleStripModel(frankfurt, 'web');
  assert.deepEqual(web.installed.map((bundle) => bundle.id), ['web.nginx_php']);
  assert.deepEqual(web.offers, []);
  // The Dashboard card: what is installed, nothing contained twice.
  const summary = lib.bundlesSummary(frankfurt);
  assert.deepEqual(summary.installed.map((bundle) => bundle.id).sort(), ['db.mariadb', 'mail.webmail', 'web.nginx_php']);
  assert.equal(summary.installable, 2, 'Spam-Protected Mail and PostgreSQL');
});

test('a host-wide refusal is said once for the category, and an unknown category claims nothing', () => {
  const dns = boston.map((bundle) => (bundle.category === 'email' ? { ...bundle, status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' } : bundle));
  assert.equal(lib.bundleStripModel(dns, 'email').hostBlocked, 'dns_identity_not_ready');
  const unknown = boston.map((bundle) => ({ ...bundle, status: 'unknown', action: 'none', blocked_code: 'state_unverified' }));
  const model = lib.bundleStripModel(unknown, 'database');
  assert.equal(model.unknown, true);
  assert.deepEqual([model.offers, model.installed, model.blocked], [[], [], []]);
  assert.equal(lib.bundlesSummary(unknown).known, false);
});

// --- Colour, shape and the one primary ------------------------------------------------
test('the strip has no filled button and no amber except a webmail runtime that is not ready', () => {
  const stripOnly = strip.slice(0, strip.indexOf('export function BundleInstallDialog'));
  assert.doesNotMatch(stripOnly, /variant='primary'|bg-primary text-primary-fg/);
  assert.equal((stripOnly.match(/text-warning/g) ?? []).length, 1);
  assert.match(stripOnly, /bundle\.warning_code === 'roundcube_not_ready'/);
  assert.doesNotMatch(stripOnly, /warning-mark|bg-warning/);
  // The wait moves (D-033): the running step draws a moving bar.
  assert.match(stripOnly, /update-step-indeterminate/);
  assert.match(stripOnly, /role='status' aria-live='polite'/);
  // The one-line notices replaced the mail boxes.
  assert.doesNotMatch(services, /data-mail-dns-secondary=''[^>]*rounded-xl border border-warning/);
});

test('a status dot has four tones, and the Components rows use them', () => {
  assert.match(ui, /export type StatusDotTone = 'success' \| 'danger' \| 'idle' \| 'unknown';/);
  assert.match(ui, /idle: 'border border-fg-subtle bg-transparent'/);
  assert.match(ui, /danger: 'bg-danger'/);
  assert.match(services, /<StatusDot tone=\{notChecked\(s\) \? 'unknown' : !s\.is_installed \? 'idle' : \(running \|\| s\.kind === 'tool' \|\| s\.status === 'installed'\) \? 'success' : 'danger'\} \/>/);
  assert.match(services, /<StatusDot tone=\{instRunning \? 'success' : 'danger'\} \/>/);
  assert.match(design, /StatusDot/);
});

test('row action icons are neutral at rest and take their tone only under the pointer or the keyboard focus', () => {
  const action = services.slice(services.indexOf('function ActionIcon('), services.indexOf('\n}\n', services.indexOf('function ActionIcon(')));
  assert.match(action, /text-fg-muted/);
  for (const line of action.split('\n').filter((entry) => /^\s+(success|danger|warning): '/.test(entry))) {
    for (const cls of line.split("'")[1].split(/\s+/)) {
      assert.match(cls, /^(hover|focus-visible):/, `"${cls}" colours an icon at rest`);
    }
  }
  // On a phone the name is written and the target is 44 px.
  assert.match(action, /max-sm:min-h-11/);
  assert.match(action, /<span className="sm:sr-only">\{title\}<\/span>/);
  // Stop is drawn as an outline; its red stays in the confirmation.
  assert.doesNotMatch(services, /<Square className="h-4 w-4" fill="currentColor"/);
  assert.match(services, /variant=\{actionName === 'stop' \? 'danger' : 'primary'\}/);
});

test('icons come from one map and the emoji is drawn nowhere', () => {
  assert.match(icons, /export function componentIcon\(id: string, category: string\): LucideIcon/);
  assert.match(icons, /componentIconMap\[id\] \?\? categoryIcons\[category\] \?\? Boxes/);
  assert.doesNotMatch(services, /\{s\.icon\}|\{service\.icon\}/);
  assert.match(services, /icon=\{componentIcon\(service\.id, service\.category\)\}/);
  assert.match(services, /<RowIcon id=\{s\.id\} category=\{s\.category\}/);
});

test('rows align in fixed columns, row Install is outlined, and a phone shows the version under the description', () => {
  assert.match(services, /md:grid md:grid-cols-\[minmax\(0,1fr\)_7rem_8rem_17rem\]/);
  assert.match(services, /const rowButtonClass = '[^']*border border-border-strong bg-surface[^']*max-sm:min-h-11[^']*'/);
  assert.doesNotMatch(services, /rounded-lg bg-primary px-3 py-1\.5 text-xs font-semibold text-primary-fg/);
  assert.match(services, /font-mono text-xs text-fg-subtle sm:hidden" data-row-version=""/);
});

test('a healthy firewall is one calm line; the box stays for every other state', () => {
  const bar = services.slice(services.indexOf('function FirewallBar('), services.indexOf('function FirewallActionConfirmationDialog('));
  assert.match(bar, /const healthy = st\.enabled\s*\n\s*&& \(st\.persistence_state === undefined \|\| st\.persistence_state === '' \|\| st\.persistence_state === 'ready'\)\s*\n\s*&& !sshReason;/);
  assert.match(bar, /if \(healthy && !expanded\) \{/);
  assert.match(bar, /data-firewall-compact=""/);
  assert.match(bar, /t\('firewall\.compact\.settings'\)/);
  // The box keeps "off" in the attention colour and is no longer green when on.
  assert.doesNotMatch(bar, /border-success\/30 bg-success\/5/);
});

test('an installed mail bundle stays visible beside a host-wide refusal; only what waits is folded into it', () => {
  const dns = boston.map((bundle) => {
    if (bundle.id === 'mail.webmail') return { ...bundle, status: 'installed_verified', action: 'none', missing_components: [], blocked_code: 'dns_identity_not_ready' };
    if (bundle.category === 'email') return { ...bundle, status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' };
    return bundle;
  });
  assert.equal(lib.decodeReadyBundles(dns, ids).length, 8);
  const email = lib.bundleStripModel(dns, 'email');
  assert.equal(email.hostBlocked, 'dns_identity_not_ready');
  assert.deepEqual(email.installed.map((bundle) => bundle.id), ['mail.webmail']);
  assert.deepEqual(email.offers, []);
});

test('a failed or running mail attempt is said again in the strip (D-024)', () => {
  const failed = boston.map((bundle) => (bundle.id === 'mail.webmail' ? { ...bundle, latest_attempt_status: 'failed' } : bundle));
  const model = lib.bundleStripModel(failed, 'email');
  assert.deepEqual(model.noted.map((bundle) => bundle.id), ['mail.webmail']);
  const strip = read('../src/components/ReadyBundleStrip.tsx');
  assert.match(strip, /if \(bundle\.latest_attempt_status === 'in_progress'\) text = t\('services\.bundles\.attemptRunning'\);/);
  assert.match(strip, /else if \(bundle\.latest_attempt_status === 'failed'\) text = \(profile \? mailProfileAttemptError\(profile, t\) : ''\) \|\| t\('services\.bundles\.lastAttemptFailed'\);/);
  assert.match(strip, /else if \(bundle\.warning_code\) text = mailProfileWarning\(/);
});
