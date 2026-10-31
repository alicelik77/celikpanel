// alpha.114: operating-system updates in the owner's hands, mounted, in
// English and Turkish. Debian and Ubuntu: the owner chooses automatic or
// manual (apt's own file); the state is what the system says; a file the
// owner edited is shown and replaced only after they confirm; a lost answer
// is unknown and sends nothing twice. Arch: no switch, step-by-step guidance
// and the PHP and PostgreSQL warnings. The Dashboard card counts what is
// waiting and never starts anything. About this system: only the system the
// panel runs on, and a new major version is never called an update.
// İşletim sistemi güncellemeleri sahibin elinde, bağlanmış olarak.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';

const en = { ...enLifecycleScreens, 'common.cancel': 'Cancel', 'common.retry': 'Try again' };
const tr = { ...trLifecycleScreens, 'common.cancel': 'Vazgeç', 'common.retry': 'Tekrar dene' };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, ShieldCheck = () => null, X = () => null, Loader2 = () => null, Server = () => null, RefreshCw = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: globalThis.cardLocale ?? 'en' });
  export const useNavigate = () => () => true;
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
  export const showToast = () => {};
  export const componentName = (s) => (typeof s === 'string' ? s : s.name);
`);
const shared = sharedLayer(stub);
const { SecurityUpdatesCard } = await import(shared.compile('components/SecurityUpdatesCard.tsx'));
const { PendingUpdatesCard, decodeOSUpdatesSummary } = await import(shared.compile('components/PendingUpdatesCard.tsx'));
const { OSReleaseCard } = await import(shared.compile('components/OSReleaseCard.tsx'));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
const AUTO_TEXT = '// Written by CelikPanel\nAPT::Periodic::Update-Package-Lists "1";\nAPT::Periodic::Unattended-Upgrade "1";\n';
const MANUAL_TEXT = '// Written by CelikPanel\nAPT::Periodic::Update-Package-Lists "1";\nAPT::Periodic::Unattended-Upgrade "0";\n';
const SHA = 'b'.repeat(64);
const config = (extra = {}) => ({
  path: '/etc/apt/apt.conf.d/20auto-upgrades', state: 'panel', mode: 'auto', current_sha256: SHA, current: AUTO_TEXT,
  legacy_path: '/etc/apt/apt.conf.d/20celikpanel-auto-upgrades', timer_state: 'enabled',
  covers: ['origin=Debian,codename=trixie,label=Debian-Security'], proposed_auto: AUTO_TEXT, proposed_manual: MANUAL_TEXT, ...extra,
});
const read = (extra = {}) => ({
  state: 'read', package_family: 'apt', automatic: { state: 'on', tool: 'unattended-upgrades' },
  last_run: '2026-10-24T06:12:01Z', last_run_source: 'log',
  pending: { state: 'known', total: 3, security: 2, security_names: ['openssl'], lists_at: '2026-10-24T05:00:00Z' },
  vendors: [], outside: [], auto_config: config(), ...extra,
});
async function mount(component, routes, { words = en, locale = 'en' } = {}) {
  globalThis.cardWords = words;
  globalThis.cardLocale = locale;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const method = init.method ?? 'GET';
    requests.push({ url, method, body: init.body });
    const route = routes[`${method} ${url}`];
    if (!route) return answer({ error: 'no route' }, 404);
    return route(init);
  };
  await act(async () => { tree = Renderer.create(React.createElement(component)); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
  delete globalThis.window;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => node.props && node.props[name] !== undefined);
const posts = () => requests.filter((request) => request.method === 'POST');
const updates = (reads, post) => ({
  'GET /api/v1/security/os-updates': (() => { let n = 0; return () => answer(reads[Math.min(n++, reads.length - 1)]); })(),
  ...(post ? { 'POST /api/v1/security/os-updates/mode': post } : {}),
});

test('the choice shows what the system says, what automatic covers, and that a new major version is not an update', async () => {
  await mount(SecurityUpdatesCard, updates([read()]));
  try {
    const all = text();
    assert.equal(attr('data-os-choice')[0].props['data-os-choice'], 'auto');
    assert.ok(all.includes(en['life.choice.now.auto']));
    assert.ok(all.includes('origin=Debian,codename=trixie,label=Debian-Security'));
    assert.ok(all.includes('By default these are the security updates'));
    assert.ok(all.includes('does not restart the server by itself'));
    assert.ok(all.includes('sudo apt-get update && sudo apt-get upgrade'));
    assert.ok(all.includes('PostgreSQL 17 after 16') && all.includes('is not an update'));
    // The option that is already in force cannot be chosen again.
    assert.equal(attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'auto').props.disabled, true);
    assert.equal(attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'manual').props.disabled, false);
    // Only reads happened: nothing was written by looking.
    assert.equal(posts().length, 0);
  } finally { await unmount(); }
});

test('switching to manual shows both texts and writes once, with the digest that was shown', async () => {
  await mount(SecurityUpdatesCard, updates([read(), read({ automatic: { state: 'off', tool: 'unattended-upgrades', reason: 'config_off' }, auto_config: config({ mode: 'manual', current: MANUAL_TEXT, current_sha256: 'c'.repeat(64) }) })], () => answer({ written: true, effective: true, mode: 'manual' })));
  try {
    await act(async () => { attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'manual').props.onClick(); });
    assert.equal(attr('data-os-choice-current')[0].props.children, AUTO_TEXT);
    assert.equal(attr('data-os-choice-text')[0].props.children, MANUAL_TEXT);
    assert.equal(attr('data-os-choice-owner-edit').length, 0);
    await act(async () => { attr('data-os-choice-confirm')[0].props.onClick(); });
    await settle();
    assert.equal(posts().length, 1);
    assert.deepEqual(JSON.parse(posts()[0].body), { mode: 'manual', expected_current_sha256: SHA, confirm_owner_edit: false });
    assert.ok(text().includes(en['life.choice.result.manual']));
    // The card read the system again, and says manual now.
    assert.equal(attr('data-os-choice')[0].props['data-os-choice'], 'manual');
    assert.equal(requests.filter((request) => request.method === 'GET').length, 2);
  } finally { await unmount(); }
});

test('a file the owner edited is announced, shown, and replaced only after the owner confirms reading it', async () => {
  const owner = 'APT::Periodic::Update-Package-Lists "3";\nAPT::Periodic::Unattended-Upgrade "1";\nAPT::Periodic::Verbose "2";\n';
  await mount(SecurityUpdatesCard, updates([read({ auto_config: config({ state: 'edited', mode: undefined, current: owner, current_sha256: 'd'.repeat(64) }) })], () => answer({ written: true, effective: true, mode: 'manual' })));
  try {
    assert.ok(text().includes('Changed by the owner'));
    assert.equal(attr('data-life-line').filter((node) => node.props['data-life-line'] === 'choice-file-edited').length, 1);
    await act(async () => { attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'manual').props.onClick(); });
    assert.equal(attr('data-os-choice-current')[0].props.children, owner);
    assert.equal(attr('data-os-choice-owner-edit').length, 1);
    // Not confirmed: the button cannot be pressed.
    assert.equal(attr('data-os-choice-confirm')[0].props.disabled, true);
    assert.equal(posts().length, 0);
    await act(async () => { attr('data-os-choice-confirm-edit')[0].props.onChange({ target: { checked: true } }); });
    assert.equal(attr('data-os-choice-confirm')[0].props.disabled, false);
    await act(async () => { attr('data-os-choice-confirm')[0].props.onClick(); });
    await settle();
    assert.deepEqual(JSON.parse(posts()[0].body), { mode: 'manual', expected_current_sha256: 'd'.repeat(64), confirm_owner_edit: true });
  } finally { await unmount(); }
});

test('a refusal says nothing was changed, who acts and what to do; the card reads the system again', async () => {
  const refusals = [
    ['OS_UPDATES_MODE_CURRENT_CHANGED', 'changed after it was shown'],
    ['OS_UPDATES_MODE_UNATTENDED_NOT_INSTALLED', 'sudo apt-get install unattended-upgrades'],
    ['OS_UPDATES_MODE_OWNER_EDITED', 'Open the choice again and confirm'],
    ['OS_UPDATES_MODE_SOMETHING_NEW', 'sudo dpkg-reconfigure -plow unattended-upgrades'],
  ];
  for (const [code, expected] of refusals) {
    await mount(SecurityUpdatesCard, updates([read()], () => answer({ error: 'the agent said no', code }, 409)));
    try {
      await act(async () => { attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'manual').props.onClick(); });
      await act(async () => { attr('data-os-choice-confirm')[0].props.onClick(); });
      await settle();
      const line = attr('data-life-line').find((node) => node.props['data-life-line'] === 'choice-result');
      assert.equal(line.props['data-tone'], 'bad', code);
      assert.ok(text().includes('Nothing was changed'), code);
      assert.ok(text().includes(expected), code);
      assert.equal(posts().length, 1);
      assert.ok(requests.filter((request) => request.method === 'GET').length >= 2, 'the system was read again');
    } finally { await unmount(); }
  }
});

test('a written choice that apt does not use is a warning with the administrator’s command', async () => {
  const cases = [
    ['overridden_by_other_file', 'grep -R "Periodic::Unattended-Upgrade" /etc/apt/apt.conf.d/'],
    ['timer_masked', 'sudo systemctl unmask apt-daily.timer apt-daily-upgrade.timer'],
    ['timer_disabled', 'sudo systemctl enable --now apt-daily.timer apt-daily-upgrade.timer'],
  ];
  for (const [reason, command] of cases) {
    await mount(SecurityUpdatesCard, updates([read()], () => answer({ written: true, effective: false, effective_reason: reason, mode: 'manual' })));
    try {
      await act(async () => { attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'manual').props.onClick(); });
      await act(async () => { attr('data-os-choice-confirm')[0].props.onClick(); });
      await settle();
      const line = attr('data-life-line').find((node) => node.props['data-life-line'] === 'choice-result');
      assert.equal(line.props['data-tone'], 'warn', reason);
      assert.ok(text().includes(command), reason);
    } finally { await unmount(); }
  }
});

test('a lost answer is unknown: it says so, sends nothing again, and reads the system', async () => {
  await mount(SecurityUpdatesCard, updates([read()], () => { throw new TypeError('network'); }));
  try {
    await act(async () => { attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'manual').props.onClick(); });
    await act(async () => { attr('data-os-choice-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(en['life.choice.unknownResult']));
    assert.equal(attr('data-life-line').find((node) => node.props['data-life-line'] === 'choice-result').props['data-tone'], 'warn');
    assert.equal(posts().length, 1);
    assert.ok(requests.filter((request) => request.method === 'GET').length >= 2);
  } finally { await unmount(); }
});

test('the owner’s symbolic link, a masked timer and an overriding file are said, with no write offered by looking', async () => {
  await mount(SecurityUpdatesCard, updates([read({ auto_config: config({ state: 'edited', unreadable: true, timer_state: 'masked', overridden: true, legacy_state: 'edited' }) })]));
  try {
    const all = text();
    assert.ok(all.includes('is not an ordinary file'));
    assert.ok(all.includes('Another configuration file overrides'));
    assert.ok(all.includes('masked'));
    assert.ok(all.includes('takes precedence over'));
    assert.equal(posts().length, 0);
  } finally { await unmount(); }
});

test('without unattended-upgrades only manual is current and automatic cannot be chosen from here', async () => {
  await mount(SecurityUpdatesCard, updates([read({ automatic: { state: 'not_installed', tool: 'unattended-upgrades' }, auto_config: config({ state: 'absent', current: undefined, current_sha256: undefined }) })]));
  try {
    assert.ok(text().includes(en['life.choice.now.notInstalled']));
    assert.equal(attr('data-os-choice-switch').find((node) => node.props['data-os-choice-switch'] === 'auto').props.disabled, true);
  } finally { await unmount(); }
});

test('Arch has no switch: the steps, the owner as the one who acts, and the PHP and PostgreSQL warnings', async () => {
  for (const [words, locale] of [[en, 'en'], [tr, 'tr']]) {
    await mount(SecurityUpdatesCard, updates([read({ package_family: 'pacman', automatic: { state: 'not_applicable', reason: 'arch_full_upgrade_by_owner' }, auto_config: undefined, last_run: '2026-10-20T08:00:01Z', pending: { state: 'known', total: 12, security: 1, security_state: 'known' } })]), { words, locale });
    try {
      assert.equal(attr('data-os-choice').length, 0);
      assert.equal(attr('data-os-choice-switch').length, 0);
      assert.equal(attr('data-arch-guide').length, 1);
      assert.deepEqual(attr('data-arch-step').map((node) => node.props['data-arch-step']), ['news', 'upgrade', 'check', 'kernel']);
      const all = text();
      assert.ok(all.includes('https://archlinux.org/news/'));
      assert.ok(all.includes('sudo pacman -Syu'));
      for (const needed of ['systemctl --failed', 'sudo nginx -t', 'php-fpm', 'uname -r', 'pacman -Q linux', 'sudo reboot', 'pg_upgrade']) assert.ok(all.includes(needed), needed);
      assert.equal(attr('data-arch-warn').length, 2);
      assert.ok(all.includes(words['life.arch.warnPhp']));
      assert.ok(all.includes(words['life.arch.warnPostgres']));
      assert.ok(all.includes(words['life.updates.auto.not_applicable']));
      assert.equal(posts().length, 0);
    } finally { await unmount(); }
  }
});

test('dnf shows neither the choice nor the Arch guide', async () => {
  await mount(SecurityUpdatesCard, updates([read({ package_family: 'dnf', auto_config: undefined })]));
  try {
    assert.equal(attr('data-os-choice').length, 0);
    assert.equal(attr('data-arch-guide').length, 0);
  } finally { await unmount(); }
});

const summary = (extra = {}) => ({
  state: 'ready', checked_at: '2026-10-31T10:00:00Z', package_family: 'apt',
  pending: { state: 'known', total: 7, security: 2, lists_at: '2026-10-31T06:00:00Z' },
  automatic: { state: 'on', tool: 'unattended-upgrades' }, ...extra,
});
const dash = (body) => ({ 'GET /api/v1/security/os-updates/summary': () => answer(body) });

test('the Dashboard card counts what is waiting, how many are security, when it was read, and links to the full page', async () => {
  await mount(PendingUpdatesCard, dash(summary()));
  try {
    const all = text();
    assert.ok(all.includes('7 waiting'));
    assert.ok(all.includes('2 of them security updates'));
    assert.ok(all.includes('Package lists of'));
    assert.ok(all.includes(en['life.pending.mode.on']));
    assert.equal(attr('data-pending-updates-link')[0].props.to, '/security');
    assert.equal(posts().length, 0);
    assert.deepEqual(requests.map((request) => request.url), ['/api/v1/security/os-updates/summary']);
  } finally { await unmount(); }
});

test('the Dashboard card: nothing waiting, Arch, manual, and an unknown reading are each said as what they are', async () => {
  await mount(PendingUpdatesCard, dash(summary({ pending: { state: 'known', total: 0, security: 0 } })));
  try { assert.ok(text().includes('Nothing is waiting.')); } finally { await unmount(); }
  await mount(PendingUpdatesCard, dash(summary({ package_family: 'pacman', automatic: { state: 'not_applicable' }, pending: { state: 'known', total: 4, security: 0, security_state: 'arch_audit_missing' } })));
  try {
    assert.ok(text().includes(en['life.pending.mode.arch']));
    assert.ok(text().includes(en['life.pending.securityUnknown']));
  } finally { await unmount(); }
  await mount(PendingUpdatesCard, dash(summary({ automatic: { state: 'off', reason: 'config_off' } })));
  try { assert.ok(text().includes(en['life.pending.mode.off'])); } finally { await unmount(); }
  await mount(PendingUpdatesCard, dash(summary({ pending: { state: 'unknown', total: 0, security: 0, reason: 'checkupdates_failed' }, package_family: 'pacman', automatic: { state: 'not_applicable' } })));
  try {
    assert.ok(text().includes(en['life.updates.pending.checkupdatesFailed']));
    assert.ok(!text().includes('Nothing is waiting.'));
  } finally { await unmount(); }
  await mount(PendingUpdatesCard, dash({ state: 'unknown' }));
  try {
    assert.ok(text().includes(en['life.pending.unknown']));
    assert.ok(!text().includes('Nothing is waiting.'));
  } finally { await unmount(); }
  await mount(PendingUpdatesCard, { 'GET /api/v1/security/os-updates/summary': () => answer({ error: 'no' }, 502) });
  try {
    assert.ok(text().includes(en['life.pending.unknown']));
  } finally { await unmount(); }
});

test('the Dashboard card while the first reading is under way says nothing needs doing and starts nothing', async () => {
  await mount(PendingUpdatesCard, dash({ state: 'checking' }));
  try {
    assert.ok(text().includes(en['life.pending.checking']));
    assert.ok(text().includes('you do not need to do anything'));
    assert.equal(posts().length, 0);
  } finally { await unmount(); }
  assert.throws(() => decodeOSUpdatesSummary({ state: 'ready' }));
  assert.throws(() => decodeOSUpdatesSummary({ state: 'sometimes' }));
  assert.equal(decodeOSUpdatesSummary({ state: 'checking' }).state, 'checking');
});

const osRead = (id, extra = {}) => ({
  state: 'read', checked: '2026-10-24', changed: false, changes: [], releases: [],
  recorded: { recorded_at: '2026-10-24T10:00:00Z' },
  current: { id, version_id: id === 'arch' ? '' : '13', pretty_name: id, level: id === 'arch' ? 'development' : 'tested', name: id, ...extra },
});

test('about this system describes only the system the panel runs on', async () => {
  for (const [id, words, locale] of [['debian', en, 'en'], ['ubuntu', en, 'en'], ['arch', en, 'en'], ['arch', tr, 'tr']]) {
    await mount(OSReleaseCard, { 'GET /api/v1/server/os-release': () => answer(osRead(id, id === 'debian' ? { patched_until: '2030-06-30' } : {})) }, { words, locale });
    try {
      assert.equal(attr('data-os-about').length, 1, id);
      assert.equal(attr('data-os-about')[0].props['data-os-about'], id);
      const all = text();
      assert.ok(all.includes(words[`life.osinfo.how.${id}`]), id);
      assert.ok(all.includes(words['life.osinfo.major']), id);
      if (id === 'arch') {
        assert.ok(all.includes(words['life.osinfo.offers.arch']));
        assert.ok(all.includes(words['life.osinfo.rolling']));
        assert.ok(words['life.osinfo.offers.arch'].includes(locale === 'en' ? 'Valkey' : 'Valkey'));
        assert.ok(words['life.osinfo.offers.arch'].includes('phpPgAdmin') && words['life.osinfo.offers.arch'].includes('SpamAssassin'));
      } else {
        assert.ok(all.includes(words['life.osinfo.offers.apt']));
        assert.ok(!all.includes(words['life.osinfo.rolling']));
      }
      // An OS that is not listed gets no description of another system's updates.
    } finally { await unmount(); }
  }
  await mount(OSReleaseCard, { 'GET /api/v1/server/os-release': () => answer(osRead('fedora')) });
  try { assert.equal(attr('data-os-about').length, 0); } finally { await unmount(); }
});

test('no new text calls a new major version an update', () => {
  for (const [name, words] of [['en', en], ['tr', tr]]) {
    for (const key of ['life.choice.major', 'life.osinfo.major']) {
      const sentence = words[key];
      assert.ok(/PostgreSQL 17/.test(sentence) && /PHP 8\.5/.test(sentence), `${name} ${key}`);
    }
  }
  assert.ok(/is not an update/.test(en['life.choice.major']) && /is a separate program and not an update/.test(en['life.osinfo.major']));
  assert.ok(/güncelleme değildir/.test(tr['life.choice.major']) && /güncelleme değildir/.test(tr['life.osinfo.major']));
});

test('every new sentence exists in English and Turkish with the same placeholders', () => {
  const keys = Object.keys(enLifecycleScreens).filter((key) => /^life\.(choice|arch|pending|osinfo)\./.test(key));
  assert.ok(keys.length > 70);
  const holes = (sentence) => [...sentence.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort().join(',');
  for (const key of keys) {
    assert.ok(trLifecycleScreens[key], `missing Turkish: ${key}`);
    assert.equal(holes(trLifecycleScreens[key]), holes(enLifecycleScreens[key]), key);
  }
  for (const key of Object.keys(trLifecycleScreens).filter((name) => /^life\.(choice|arch|pending|osinfo)\./.test(name))) assert.ok(enLifecycleScreens[key], `missing English: ${key}`);
});
